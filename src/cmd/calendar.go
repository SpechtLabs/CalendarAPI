package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/sierrasoftworks/humane-errors-go"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/SpechtLabs/CalendarAPI/pkg/client"
	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
)

var outFormat string

var clearCalendarCmd = &cobra.Command{
	Use:     "calendar",
	Example: "meetingepd clear calendar",
	Long:    "Clear the cache of the server and force it to fetch the latest info from the iCal",
	Args:    cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		err := withCalendarAPI(cmd.Context(), 10*time.Second, func(ctx context.Context, api pb.CalenderServiceClient) error {
			_, err := api.RefreshCalendar(ctx, &pb.CalendarRequest{CalendarName: client.AllCalendars})
			return err
		})
		if err != nil {
			return err
		}

		fmt.Print("Cleared calendar cache\n")
		return nil
	},
}

var getCalendarCmd = &cobra.Command{
	Use:     "calendar [calendar_name]",
	Example: "meetingepd get calendar",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		calendarName := client.AllCalendars
		if len(args) == 1 {
			calendarName = args[0]
		}

		var calendar *pb.CalendarResponse
		err := withCalendarAPI(cmd.Context(), time.Second, func(ctx context.Context, api pb.CalenderServiceClient) error {
			var err error
			calendar, err = api.GetCalendar(ctx, &pb.CalendarRequest{CalendarName: calendarName})
			return err
		})
		if err != nil {
			return err
		}

		out, err := renderCalendar(calendar, outFormat, time.Now())
		if err != nil {
			return err
		}

		fmt.Println(out)
		return nil
	},
}

// textStyles are the styles of the text output of `get calendar`.
type textStyles struct {
	header, context, important, free, tentative, outOfOffice, normal, past lipgloss.Style
}

func addCalendarCommands() {
	getCalendarCmd.Flags().StringVarP(&outFormat, "out", "o", "text", "Configure your output format (text, json, yaml)")

	clearCmd.AddCommand(clearCalendarCmd)
	getCmd.AddCommand(getCalendarCmd)
}

// renderCalendar renders the calendar as JSON, YAML, or text for a terminal.
// The text marks the events that ended before now as past.
func renderCalendar(calendar *pb.CalendarResponse, format string, now time.Time) (string, humane.Error) {
	switch format {
	case "json":
		out, err := json.Marshal(calendar)
		if err != nil {
			return "", humane.Wrap(err, "failed to render the calendar as JSON", "use --out text or --out yaml instead")
		}
		return string(out), nil

	case "yaml":
		out, err := yaml.Marshal(calendar)
		if err != nil {
			return "", humane.Wrap(err, "failed to render the calendar as YAML", "use --out text or --out json instead")
		}
		return string(out), nil

	default:
		return formatText(calendar, now), nil
	}
}

func newTextStyles() textStyles {
	return textStyles{
		header:      lipgloss.NewStyle().Bold(true).Underline(true),
		context:     lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#999999")),
		important:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true),
		free:        lipgloss.NewStyle().Foreground(lipgloss.Color("#999999")),
		tentative:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Italic(true),
		outOfOffice: lipgloss.NewStyle().Foreground(lipgloss.Color("#800080")).Bold(true),
		normal:      lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")),
		past:        lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("#666666")),
	}
}

func formatText(resp *pb.CalendarResponse, now time.Time) string {
	styles := newTextStyles()

	var out strings.Builder
	out.WriteString(styles.context.Render(fmt.Sprintf("(last refreshed: %s)", time.Unix(resp.LastUpdated, 0).Format(time.TimeOnly))))
	out.WriteString("\n\n")

	fmt.Fprintf(&out, "Calendar: %s Date: %s",
		styles.header.Render(resp.CalendarName),
		styles.header.Render(time.Unix(resp.LastUpdated, 0).Format(time.DateOnly)),
	)

	out.WriteString("\n")

	// Separate all-day from timed events
	allDayEntries := []*pb.CalendarEntry{}
	normalEntries := []*pb.CalendarEntry{}
	showCalendarName := false
	for _, e := range resp.Entries {
		if e.CalendarName != resp.CalendarName {
			showCalendarName = true
		}
		if e.AllDay {
			allDayEntries = append(allDayEntries, e)
		} else {
			normalEntries = append(normalEntries, e)
		}
	}

	// Show all-day first, then timed events
	for idx, item := range append(allDayEntries, normalEntries...) {
		out.WriteString(renderEntry(item, idx+1, now, showCalendarName, styles))
	}

	return out.String()
}

func renderEntry(item *pb.CalendarEntry, idx int, now time.Time, showCalendarName bool, styles textStyles) string {
	if item == nil {
		return ""
	}

	start := time.Unix(item.Start, 0)
	end := time.Unix(item.End, 0)

	// Base line (without styling yet)
	var line string

	// 1. Add Index
	line = fmt.Sprintf("%2d) ", idx)

	// 2. Add status (only for tentative, OOO, or working elsewhere)
	switch item.Busy {
	case pb.BusyState_Tentative:
		fallthrough
	case pb.BusyState_OutOfOffice:
		fallthrough
	case pb.BusyState_WorkingElsewhere:
		line += fmt.Sprintf("[%s]", item.Busy.String())
	}

	if item.AllDay {
		line += fmt.Sprintf("%s (all day)", item.Title)
	} else {
		line += fmt.Sprintf("%s: <%s - %s>", item.Title, start.Format(time.Kitchen), end.Format(time.Kitchen))
	}

	if len(item.Message) > 0 {
		line += fmt.Sprintf(" - %s", item.Message)
	}

	// Past event? Strike through
	if end.Before(now) {
		return styles.past.Render(line) + styles.past.Italic(true).Render(fmt.Sprintf(" (%s)", item.CalendarName)) + "\n"
	}

	// Apply styles based on attributes
	switch {
	case item.Important:
		line = styles.important.Render(line)
	case item.Busy == pb.BusyState_Free:
		line = styles.free.Render(line)
	case item.Busy == pb.BusyState_Tentative:
		line = styles.tentative.Render(line)
	case item.Busy == pb.BusyState_OutOfOffice || item.Busy == pb.BusyState_WorkingElsewhere:
		line = styles.outOfOffice.Render(line)
	default:
		line = styles.normal.Render(line)
	}

	if showCalendarName {
		line += styles.context.Render(fmt.Sprintf(" (%s)", item.CalendarName))
	}

	return line + "\n"
}
