package client

import (
	"fmt"
	"strconv"
	"strings"

	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
	"github.com/spechtlabs/go-otel-utils/otelzap"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Rule struct {
	CalendarName string   `mapstructure:"calendar"`
	Name         string   `mapstructure:"name"`
	Key          string   `mapstructure:"key"`
	Contains     []string `mapstructure:"contains"`
	Skip         bool     `mapstructure:"skip"`
	Message      string   `mapstructure:"message"`
	Important    bool     `mapstructure:"important"`
}

// Evaluate evaluates a rule against a pb.CalendarEntry and returns (bool, bool)
// where the first bool indicates if the rule was applied to this pb.CalendarEntry
// and the second bool indicates if this is a skip rule and the pb.CalendarEntry
// should be skipped
func (r *Rule) Evaluate(e *pb.CalendarEntry) (bool, bool) {
	var matchFieldValue string
	var matchFieldContains string
	match := false

	// only evaluate our rule if the calendar matches
	if r.CalendarName == "" || r.CalendarName == "*" || r.CalendarName == "all" || r.CalendarName == e.CalendarName {
		switch r.Key {
		case "title":
			matchFieldValue = e.Title

		case "all_day":
			matchFieldValue = strconv.FormatBool(e.AllDay)

		case "busy":
			matchFieldValue = e.Busy.String()

			// if the user wants to match on all possible locations,
			// let's just concatenate them all in one big string, shall we?
			// This way we search all fields :D
		case "*":
			matchFieldValue = fmt.Sprintf("%s%s%s", e.Title, strconv.FormatBool(e.AllDay), e.Busy.String())
		}

		for _, contains := range r.Contains {
			if contains == "*" {
				match = true
			}

			// compare but ignore case...
			if strings.Contains(strings.ToLower(matchFieldValue), strings.ToLower(contains)) {
				match = true
			}

			if match {
				matchFieldContains = contains
				break
			}
		}
	}

	// The rule doesn't match, so we also don't skip
	if !match {
		return false, false
	}

	// perform the relabelings
	if e.Message != r.Message {
		e.Message = r.Message
	}

	if e.Important != r.Important {
		e.Important = r.Important
	}

	otelzap.L().Sugar().Debugw("Rule Evaluated",
		zap.String("rule_name", r.Name),
		zap.String("calendar_name", r.CalendarName),
		zap.String("title", e.Title),
		zap.String("key", r.Key),
		zap.String("Field", matchFieldValue),
		zap.String("contains", matchFieldContains),
		zap.Bool("skip", r.Skip),
		zap.Bool("relabel_important", e.Important),
		zap.String("relabel_message", e.Message),
	)

	return true, r.Skip
}

func parseRules() []Rule {
	var rules []Rule
	err := viper.UnmarshalKey("rules", &rules)
	if err != nil {
		otelzap.L().WithError(err).Error("Failed to parse rules")
		return nil
	}
	return rules
}
