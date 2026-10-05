package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
)

// statusUse names the status command under get, set and clear.
const statusUse = "status"

var (
	calendar    string
	description string
	icon        string
	iconSize    int32
)

var getCustomStatusCmd = &cobra.Command{
	Use:     statusUse,
	Example: "meetingepd get status",
	Args:    cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		var customStatus *pb.CustomStatus
		err := withCalendarAPI(cmd.Context(), time.Second, func(ctx context.Context, api pb.CalenderServiceClient) error {
			var err error
			customStatus, err = api.GetCustomStatus(ctx, &pb.GetCustomStatusRequest{CalendarName: calendar})
			return err
		})
		if err != nil {
			return err
		}

		printCustomStatus("Custom Status:", customStatus)
		return nil
	},
}

var setCustomStatusCmd = &cobra.Command{
	Use:     statusUse,
	Example: "meetingepd set status",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var customStatus *pb.CustomStatus
		err := withCalendarAPI(cmd.Context(), time.Second, func(ctx context.Context, api pb.CalenderServiceClient) error {
			var err error
			customStatus, err = api.SetCustomStatus(ctx, &pb.SetCustomStatusRequest{
				CalendarName: calendar,
				Status: &pb.CustomStatus{
					Title:       args[0],
					Description: description,
					Icon:        icon,
					IconSize:    iconSize,
				},
			})
			return err
		})
		if err != nil {
			return err
		}

		printCustomStatus("Set Custom Status:", customStatus)
		return nil
	},
}

var clearCustomStatusCmd = &cobra.Command{
	Use:     statusUse,
	Example: "meetingepd clear status",
	Args:    cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		err := withCalendarAPI(cmd.Context(), time.Second, func(ctx context.Context, api pb.CalenderServiceClient) error {
			_, err := api.ClearCustomStatus(ctx, &pb.ClearCustomStatusRequest{CalendarName: calendar})
			return err
		})
		if err != nil {
			return err
		}

		fmt.Print("Cleared Custom Status\n")
		return nil
	},
}

func addCustomStatusCommands() {
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

// printCustomStatus prints the status under the heading, or that it is not
// set.
func printCustomStatus(heading string, customStatus *pb.CustomStatus) {
	if customStatus == nil {
		customStatus = &pb.CustomStatus{}
	}

	fmt.Print(heading)
	if len(customStatus.GetTitle()) == 0 {
		fmt.Printf(" is not set\n")
		return
	}

	fmt.Printf("\n")
	fmt.Printf("  - Title: %s\n", customStatus.GetTitle())
	fmt.Printf("  - Description: %s\n", customStatus.GetDescription())
	fmt.Printf("  - Icon: %s (%dx%d)\n", customStatus.GetIcon(), customStatus.GetIconSize(), customStatus.GetIconSize())
}
