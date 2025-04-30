package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/SpechtLabs/CalendarAPI/pkg/api"
	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
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
	outStr := fmt.Sprintf("Got Calendar (last refreshed: %s)\n\n", time.Unix(resp.LastUpdated, 0).Format(time.RFC822))

	for idx, item := range resp.Entries {
		outStr += fmt.Sprintf("%d) ", idx)

		if item.Important {
			outStr += "!"
		}

		outStr += fmt.Sprintf("%s: [%s to %s] - %s", item.Title, time.Unix(item.Start, 0).Format(time.RFC822), time.Unix(item.End, 0).Format(time.RFC822), item.Busy.String())

		if item.AllDay {
			outStr += " (all day)"
		}

		if len(item.Message) > 0 {
			outStr += fmt.Sprintf(": %s", item.Message)
		}

		outStr += "\n"
	}

	return outStr
}

func init() {
	getCalendarCmd.Flags().StringVarP(&outFormat, "out", "o", "text", "Configure your output format (text, json, yaml)")

	clearCmd.AddCommand(clearCalendarCmd)
	getCmd.AddCommand(getCalendarCmd)
}
