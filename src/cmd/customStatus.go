package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/SpechtLabs/CalendarAPI/pkg/api"
	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
	"github.com/spechtlabs/go-otel-utils/otelzap"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

var (
	calendar    string
	description string
	icon        string
	iconSize    int32
)

var getCustomStatusCmd = &cobra.Command{
	Use:     "status",
	Example: "meetingepd get status",
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		customStatus, err := client.GetCustomStatus(ctx, &pb.GetCustomStatusRequest{CalendarName: calendar})
		if err != nil {
			otelzap.L().Fatal(fmt.Sprintf("Failed to talk to gRPC API (%s) %v", addr, err))
		}

		fmt.Print("Custom Status:")
		if len(customStatus.Title) > 0 {
			fmt.Printf("\n")
			fmt.Printf("  - Title: %s\n", customStatus.Title)
			fmt.Printf("  - Description: %s\n", customStatus.Description)
			fmt.Printf("  - Icon: %s (%dx%d)\n", customStatus.Icon, customStatus.IconSize, customStatus.IconSize)
		} else {
			fmt.Printf(" is not set\n")
		}
	},
}

var setCustomStatusCmd = &cobra.Command{
	Use:     "status",
	Example: "meetingepd set status",
	Args:    cobra.ExactArgs(1),
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		customStatus, err := client.SetCustomStatus(ctx, &pb.SetCustomStatusRequest{
			CalendarName: calendar,
			Status: &pb.CustomStatus{
				Title:       args[0],
				Description: description,
				Icon:        icon,
				IconSize:    iconSize,
			},
		})
		if err != nil {
			otelzap.L().Fatal(fmt.Sprintf("Failed to talk to gRPC API (%s) %v", addr, err))
		}

		fmt.Print("Set Custom Status:")
		if len(customStatus.Title) > 0 {
			fmt.Printf("\n")
			fmt.Printf("  - Title: %s\n", customStatus.Title)
			fmt.Printf("  - Description: %s\n", customStatus.Description)
			fmt.Printf("  - Icon: %s (%dx%d)\n", customStatus.Icon, customStatus.IconSize, customStatus.IconSize)
		} else {
			fmt.Printf(" is not set\n")
		}
	},
}

var clearCustomStatusCmd = &cobra.Command{
	Use:     "status",
	Example: "meetingepd clear status",
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
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		_, err := client.ClearCustomStatus(ctx, &pb.ClearCustomStatusRequest{CalendarName: calendar})
		if err != nil {
			otelzap.L().Fatal(fmt.Sprintf("Failed to talk to gRPC API (%s) %v", addr, err))
		}

		fmt.Print("Cleared Custom Status\n")
	},
}

func init() {
	setCustomStatusCmd.Flags().StringVarP(&description, "description", "t", "", "Description of your custom status")
	setCustomStatusCmd.Flags().StringVarP(&icon, "icon", "i", "warning_icon", "Icon to use in custom status")
	setCustomStatusCmd.Flags().Int32Var(&iconSize, "icon_size", 196, "Icon size to display in the custom status")

	setCustomStatusCmd.Flags().StringVarP(&calendar, "calendar", "q", "", "Name of the calendar to set the custom status for")
	_ = setCustomStatusCmd.MarkFlagRequired("calendar")

	getCustomStatusCmd.Flags().StringVarP(&calendar, "calendar", "q", "", "Name of the calendar to set the custom status for")
	_ = getCustomStatusCmd.MarkFlagRequired("calendar")

	clearCustomStatusCmd.Flags().StringVarP(&calendar, "calendar", "q", "", "Name of the calendar to set the custom status for")
	_ = clearCustomStatusCmd.MarkFlagRequired("calendar")

	setCmd.AddCommand(setCustomStatusCmd)
	getCmd.AddCommand(getCustomStatusCmd)
	clearCmd.AddCommand(clearCustomStatusCmd)
}
