package cmd

import (
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:     "get",
	Example: "meetingepd get",
	Run: func(cmd *cobra.Command, args []string) {
	},
}

var setCmd = &cobra.Command{
	Use:     "set",
	Example: "meetingepd set",
	Run: func(cmd *cobra.Command, args []string) {
	},
}

var clearCmd = &cobra.Command{
	Use:     "clear",
	Example: "meetingepd clear",
	Run: func(cmd *cobra.Command, args []string) {
	},
}

func init() {
	rootCmd.AddCommand(clearCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(setCmd)
}
