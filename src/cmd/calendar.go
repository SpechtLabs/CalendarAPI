package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/SpechtLabs/CalendarAPI/pkg/api"
	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
	"github.com/charmbracelet/lipgloss"
	"github.com/spechtlabs/go-otel-utils/otelzap"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v3"
)

var outFormat string

var clearCalendarCmd = &cobra.Command{
	Use:     "calendar",
	Example: "meetingepd clear calendar",
	Long:    "Clear the cache of the server and force it to fetch the latest info from the iCal",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		addr := fmt.Sprintf("%s:%d", hostname, grpcPort)

		conn, client := api.NewGrpcApiClient(addr)
		defer func(conn *grpc.ClientConn) {
			err := conn.Close()
			if err != nil {
				otelzap.L().Sugar().Errorw("failed to close gRPC connection", zap.Error(err))
			}
		}(conn)

		// Contact the server
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := client.RefreshCalendar(ctx, &pb.CalendarRequest{CalendarName: "all"})
		if err != nil {
			otelzap.L().Fatal(fmt.Sprintf("Failed to talk to gRPC API (%s) %v", addr, err))
		}

		fmt.Print("Cleared calendar cache\n")
	},
}

var getCalendarCmd = &cobra.Command{
	Use:     "calendar [calendar_name]",
	Example: "meetingepd get calendar",
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		calendarName := "all"
		if len(args) == 1 {
			calendarName = args[0]
		}

		addr := fmt.Sprintf("%s:%d", hostname, grpcPort)

		conn, client := api.NewGrpcApiClient(addr)
		defer func(conn *grpc.ClientConn) {
			err := conn.Close()
			if err != nil {
				otelzap.L().Sugar().Errorw("failed to close gRPC connection", zap.Error(err))
			}
		}(conn)

		// Contact the server
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		calendar, err := client.GetCalendar(ctx, &pb.CalendarRequest{CalendarName: calendarName})
		if err != nil {
			otelzap.L().Fatal(fmt.Sprintf("Failed to talk to gRPC API (%s) %v", addr, err))
		}

		switch outFormat {
		case "json":
			json, err := json.Marshal(calendar)
			if err != nil {
				otelzap.L().Sugar().Error("failed to parse calendar config", zap.Error(err))
			}
			fmt.Println(string(json))

		case "yaml":
			yaml, err := yaml.Marshal(calendar)
			if err != nil {
				otelzap.L().Sugar().Error("failed to parse calendar config", zap.Error(err))
			}
			fmt.Println(string(yaml))

		default:
			fmt.Println(formatText(calendar))
		}
	},
}

func formatText(resp *pb.CalendarResponse) string {
	now := time.Now()

	// Styles
	headerStyle := lipgloss.NewStyle().Bold(true).Underline(true)
	contextStyle := lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#999999"))
	importantStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)
	freeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))
	tentativeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500")).Italic(true)
	outOfOfficeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#800080")).Bold(true)
	defaultStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	strikeThroughStyle := lipgloss.NewStyle().Strikethrough(true).Foreground(lipgloss.Color("#666666"))

	outStr := ""
	outStr += contextStyle.Render(fmt.Sprintf("(last refreshed: %s)", time.Unix(resp.LastUpdated, 0).Format(time.TimeOnly)))
	outStr += "\n\n"

	outStr += fmt.Sprintf("Calendar: %s Date: %s",
		headerStyle.Render(resp.CalendarName),
		headerStyle.Render(time.Unix(resp.LastUpdated, 0).Format(time.DateOnly)),
	)

	outStr += "\n"

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

	idx := 1
	// Show all-day first
	for _, item := range allDayEntries {
		outStr += renderEntry(item, idx, now, showCalendarName, strikeThroughStyle, importantStyle,
			freeStyle, tentativeStyle, outOfOfficeStyle, defaultStyle, contextStyle)
		idx++
	}

	// Then timed events
	for _, item := range normalEntries {
		outStr += renderEntry(item, idx, now, showCalendarName, strikeThroughStyle, importantStyle,
			freeStyle, tentativeStyle, outOfOfficeStyle, defaultStyle, contextStyle)
		idx++
	}

	return outStr
}

func renderEntry(
	item *pb.CalendarEntry,
	idx int,
	now time.Time,
	showCalendarName bool,
	strikeThroughStyle, importantStyle, freeStyle,
	tentativeStyle, outOfOfficeStyle, defaultStyle,
	contextStyle lipgloss.Style,
) string {
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
		return strikeThroughStyle.Render(line) + strikeThroughStyle.Italic(true).Render(fmt.Sprintf(" (%s)", item.CalendarName)) + "\n"
	}

	// Apply styles based on attributes
	switch {
	case item.Important:
		line = importantStyle.Render(line)
	case item.Busy == pb.BusyState_Free:
		line = freeStyle.Render(line)
	case item.Busy == pb.BusyState_Tentative:
		line = tentativeStyle.Render(line)
	case item.Busy == pb.BusyState_OutOfOffice || item.Busy == pb.BusyState_WorkingElsewhere:
		line = outOfOfficeStyle.Render(line)
	default:
		line = defaultStyle.Render(line)
	}

	if showCalendarName {
		line += contextStyle.Render(fmt.Sprintf(" (%s)", item.CalendarName))
	}

	return line + "\n"
}

func init() {
	getCalendarCmd.Flags().StringVarP(&outFormat, "out", "o", "text", "Configure your output format (text, json, yaml)")

	clearCmd.AddCommand(clearCalendarCmd)
	getCmd.AddCommand(getCalendarCmd)
}
