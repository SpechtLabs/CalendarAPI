package client

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sierrasoftworks/humane-errors-go"
	"github.com/spechtlabs/go-otel-utils/otelzap"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
)

// Rule is one entry of the rules config. A rule matches an event of its
// calendar (or of every calendar) whose Key field contains one of the
// Contains strings, ignoring case, and then either skips the event or
// relabels it with Message and Important.
type Rule struct {
	CalendarName string   `mapstructure:"calendar"`
	Name         string   `mapstructure:"name"`
	Key          string   `mapstructure:"key"`
	Message      string   `mapstructure:"message"`
	Contains     []string `mapstructure:"contains"`
	Skip         bool     `mapstructure:"skip"`
	Important    bool     `mapstructure:"important"`
}

// Evaluate evaluates a rule against a pb.CalendarEntry and returns (bool, bool)
// where the first bool indicates if the rule was applied to this pb.CalendarEntry
// and the second bool indicates if this is a skip rule and the pb.CalendarEntry
// should be skipped
func (r *Rule) Evaluate(e *pb.CalendarEntry) (bool, bool) {
	if e == nil {
		return false, false
	}

	var matchFieldValue string
	var matchFieldContains string
	match := false

	// only evaluate our rule if the calendar matches
	if r.CalendarName == "" || r.CalendarName == "*" || r.CalendarName == AllCalendars || r.CalendarName == e.CalendarName {
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

func parseRules() ([]Rule, humane.Error) {
	var rules []Rule
	if err := viper.UnmarshalKey("rules", &rules); err != nil {
		return nil, humane.Wrap(err, "failed to parse the rules config",
			"check the rules section of the config file against the documentation")
	}

	return rules, nil
}
