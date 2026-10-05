package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Shows version information",
	Example: "meetingepd version",
	Args:    cobra.ExactArgs(0),
	// The version needs neither a config file nor telemetry, so this replaces
	// the root command's hooks that set them up.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	PersistentPostRun: func(cmd *cobra.Command, args []string) {},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Version: %s\n", Version)
		fmt.Printf("Date:    %s\n", Date)
		fmt.Printf("Commit:  %s\n", Commit)
		fmt.Printf("BuiltBy: %s\n", BuiltBy)
	},
}

func addVersionCommand() {
	rootCmd.AddCommand(versionCmd)
}
