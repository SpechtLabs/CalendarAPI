package client

import (
	"testing"

	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
)

func TestRuleEvaluate(t *testing.T) {
	standup := func() *pb.CalendarEntry {
		return &pb.CalendarEntry{Title: "Daily Standup", CalendarName: "work", Busy: pb.BusyState_Busy}
	}

	tests := []struct {
		name          string
		rule          Rule
		entry         *pb.CalendarEntry
		wantMatch     bool
		wantSkip      bool
		wantMessage   string
		wantImportant bool
	}{
		{
			name:        "title contains, ignoring case",
			rule:        Rule{Key: "title", Contains: []string{"STANDUP"}, Message: "Standup"},
			entry:       standup(),
			wantMatch:   true,
			wantMessage: "Standup",
		},
		{
			name:  "title doesn't contain",
			rule:  Rule{Key: "title", Contains: []string{"Retro"}, Message: "Retro"},
			entry: standup(),
		},
		{
			name:          "wildcard contains matches anything",
			rule:          Rule{Key: "title", Contains: []string{"*"}, Important: true},
			entry:         standup(),
			wantMatch:     true,
			wantImportant: true,
		},
		{
			name:      "skip rule",
			rule:      Rule{Key: "title", Contains: []string{"standup"}, Skip: true},
			entry:     standup(),
			wantMatch: true,
			wantSkip:  true,
		},
		{
			name:      "busy state",
			rule:      Rule{Key: "busy", Contains: []string{"busy"}},
			entry:     standup(),
			wantMatch: true,
		},
		{
			name:      "all day",
			rule:      Rule{Key: "all_day", Contains: []string{"true"}},
			entry:     &pb.CalendarEntry{Title: "Holiday", AllDay: true},
			wantMatch: true,
		},
		{
			name:      "any field",
			rule:      Rule{Key: "*", Contains: []string{"false"}},
			entry:     standup(),
			wantMatch: true,
		},
		{
			name:      "the rule's calendar",
			rule:      Rule{CalendarName: "work", Key: "title", Contains: []string{"standup"}},
			entry:     standup(),
			wantMatch: true,
		},
		{
			name:      "every calendar",
			rule:      Rule{CalendarName: AllCalendars, Key: "title", Contains: []string{"standup"}},
			entry:     standup(),
			wantMatch: true,
		},
		{
			name:  "another calendar",
			rule:  Rule{CalendarName: "home", Key: "title", Contains: []string{"standup"}},
			entry: standup(),
		},
		{
			name:  "unknown key",
			rule:  Rule{Key: "location", Contains: []string{"standup"}},
			entry: standup(),
		},
		{
			name:  "nil entry",
			rule:  Rule{Key: "title", Contains: []string{"*"}},
			entry: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, skip := tt.rule.Evaluate(tt.entry)
			if match != tt.wantMatch || skip != tt.wantSkip {
				t.Fatalf("Evaluate() = (%v, %v), want (%v, %v)", match, skip, tt.wantMatch, tt.wantSkip)
			}
			if tt.entry == nil {
				return
			}
			if tt.entry.Message != tt.wantMessage || tt.entry.Important != tt.wantImportant {
				t.Errorf("entry relabeled to (%q, %v), want (%q, %v)",
					tt.entry.Message, tt.entry.Important, tt.wantMessage, tt.wantImportant)
			}
		})
	}
}

func TestApplyRules(t *testing.T) {
	tests := []struct {
		name        string
		rules       []Rule
		wantKeep    bool
		wantMessage string
	}{
		{
			name:  "no rules drop every event",
			rules: nil,
		},
		{
			name:  "no matching rule drops the event",
			rules: []Rule{{Key: "title", Contains: []string{"Retro"}}},
		},
		{
			name: "the first matching rule relabels",
			rules: []Rule{
				{Key: "title", Contains: []string{"Retro"}, Message: "retro"},
				{Key: "title", Contains: []string{"Standup"}, Message: "first"},
				{Key: "title", Contains: []string{"*"}, Message: "second"},
			},
			wantKeep:    true,
			wantMessage: "first",
		},
		{
			name: "a matching skip rule drops the event",
			rules: []Rule{
				{Key: "title", Contains: []string{"Standup"}, Skip: true},
				{Key: "title", Contains: []string{"*"}, Message: "catch-all"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &pb.CalendarEntry{Title: "Standup"}
			if keep := applyRules(entry, tt.rules); keep != tt.wantKeep {
				t.Fatalf("applyRules() = %v, want %v", keep, tt.wantKeep)
			}
			if entry.Message != tt.wantMessage {
				t.Errorf("message = %q, want %q", entry.Message, tt.wantMessage)
			}
		})
	}
}
