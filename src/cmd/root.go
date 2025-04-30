package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// Version represents the Version of the kkpctl binary, should be set via ldflags -X
	Version string

	// Date represents the Date of when the kkpctl binary was build, should be set via ldflags -X
	Date string

	// Commit represents the Commit-hash from which kkpctl binary was build, should be set via ldflags -X
	Commit string

	// BuiltBy represents who build the binary, should be set via ldflags -X
	BuiltBy string

	hostname               string
	grpcPort               int
	restPort               int
	defaultCalendarRefresh time.Duration = 30 * time.Minute
	configFileName         string
	debug                  bool
)

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&configFileName, "config", "c", "", "Name of the config file")

	rootCmd.PersistentFlags().BoolP("debug", "d", false, "enable debug logging")
	viper.SetDefault("server.debug", false)
	err := viper.BindPFlag("server.debug", rootCmd.PersistentFlags().Lookup("debug"))
	if err != nil {
		panic(fmt.Errorf("fatal binding flag: %w", err))
	}

	rootCmd.PersistentFlags().IntVar(&restPort, "restPort", 50051, "Port of the gRPC API of the Server")
	viper.SetDefault("server.httpPort", 8099)
	err = viper.BindPFlag("server.httpPort", rootCmd.PersistentFlags().Lookup("restPort"))
	if err != nil {
		panic(fmt.Errorf("fatal binding flag: %w", err))
	}

	rootCmd.PersistentFlags().IntVar(&grpcPort, "grpcPort", 50051, "Port of the gRPC API of the Server")
	viper.SetDefault("server.grpcPort", 50051)
	err = viper.BindPFlag("server.grpcPort", rootCmd.PersistentFlags().Lookup("grpcPort"))
	if err != nil {
		panic(fmt.Errorf("fatal binding flag: %w", err))
	}

	rootCmd.PersistentFlags().StringVarP(&hostname, "server", "s", "", "Port of the gRPC API of the Server")
	viper.SetDefault("server.host", "")
	err = viper.BindPFlag("server.host", rootCmd.PersistentFlags().Lookup("server"))
	if err != nil {
		panic(fmt.Errorf("fatal binding flag: %w", err))
	}
}

func initConfig() {
	if configFileName != "" {
		viper.SetConfigFile(configFileName)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath(home)
		viper.AddConfigPath("$HOME/.config/calendarapi/")
		viper.AddConfigPath("/data")
	}

	viper.SetEnvPrefix("CALAPI")
	viper.AutomaticEnv()

	// Find and read the config file
	if err := viper.ReadInConfig(); err != nil {
		// Handle errors reading the config file
		panic(fmt.Errorf("fatal error config file: %w", err))
	}

	hostname = viper.GetString("server.host")
	grpcPort = viper.GetInt("server.grpcPort")
	restPort = viper.GetInt("server.httpPort")
	debug = viper.GetBool("server.debug")
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "meetingepd",
	Short: "A CLI for interacting with the meetingroom epd dipslay server.",
	Long:  `This is a CLI for interacting with the meetingroom epd display server`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(version, commit, date, builtBy string) {
	// asign build flags for version info
	Version = version
	Date = date
	Commit = commit
	BuiltBy = builtBy

	err := rootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
