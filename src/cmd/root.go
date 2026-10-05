package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sierrasoftworks/humane-errors-go"
	"github.com/spechtlabs/go-otel-utils/otelprovider"
	"github.com/spechtlabs/go-otel-utils/otelzap"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	// Version represents the Version of the calendarapi binary, should be set via ldflags -X
	Version string

	// Date represents the Date of when the calendarapi binary was build, should be set via ldflags -X
	Date string

	// Commit represents the Commit-hash from which calendarapi binary was build, should be set via ldflags -X
	Commit string

	// BuiltBy represents who build the binary, should be set via ldflags -X
	BuiltBy string

	hostname               string
	grpcPort               int
	restPort               int
	defaultCalendarRefresh = 30 * time.Minute
	configFileName         string
	debug                  bool
)

var undoFunc func()

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "meetingepd",
	Short: "A CLI for interacting with the meetingroom epd dipslay server.",
	Long:  `This is a CLI for interacting with the meetingroom epd display server`,
	// Execute prints the error, with its advice, itself.
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if err := initConfig(); err != nil {
			return err
		}
		undoFunc = initO11y()
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		undoFunc()
	},
}

// Execute builds the command tree and runs it. main calls it once.
func Execute(version, commit, date, builtBy string) {
	// assign build flags for version info
	Version = version
	Date = date
	Commit = commit
	BuiltBy = builtBy

	if err := addRootFlags(); err != nil {
		humane.Eprint(err)
		os.Exit(1)
	}

	addVerbCommands()
	addCalendarCommands()
	addCustomStatusCommands()
	addServeCommand()
	addVersionCommand()

	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		humane.Eprint(err)
		os.Exit(1)
	}
}

func addRootFlags() error {
	rootCmd.PersistentFlags().StringVarP(&configFileName, "config", "c", "", "Name of the config file")

	rootCmd.PersistentFlags().BoolP("debug", "d", false, "enable debug logging")
	viper.SetDefault("server.debug", false)

	rootCmd.PersistentFlags().IntVar(&restPort, "restPort", 8099, "Port of the REST API of the Server")
	viper.SetDefault("server.httpPort", 8099)

	rootCmd.PersistentFlags().IntVar(&grpcPort, "grpcPort", 50051, "Port of the gRPC API of the Server")
	viper.SetDefault("server.grpcPort", 50051)

	rootCmd.PersistentFlags().StringVarP(&hostname, "server", "s", "", "Host name or address of the Server")
	viper.SetDefault("server.host", "")

	return errors.Join(
		bindFlag("server.debug", "debug"),
		bindFlag("server.httpPort", "restPort"),
		bindFlag("server.grpcPort", "grpcPort"),
		bindFlag("server.host", "server"),
	)
}

func bindFlag(key, flag string) error {
	if err := viper.BindPFlag(key, rootCmd.PersistentFlags().Lookup(flag)); err != nil {
		return humane.Wrap(err, fmt.Sprintf("failed to bind the --%s flag to %s", flag, key),
			"this is a bug in calendarapi; please report it")
	}

	return nil
}

func initConfig() humane.Error {
	if configFileName != "" {
		viper.SetConfigFile(configFileName)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return humane.Wrap(err, "failed to find the home directory to look for config.yaml in",
				"pass the config file with --config")
		}

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
		return humane.Wrap(err, "failed to read the config file",
			"pass the config file with --config, or put config.yaml in the working directory, the home directory, ~/.config/calendarapi/ or /data")
	}

	hostname = viper.GetString("server.host")
	grpcPort = viper.GetInt("server.grpcPort")
	restPort = viper.GetInt("server.httpPort")
	debug = viper.GetBool("server.debug")

	return nil
}

func initO11y() func() {
	var loggerOptions []otelprovider.LoggerOption
	var tracerOptions []otelprovider.TracerOption

	otelEndpoint := viper.GetString("otel.endpoint")
	otelInsecure := viper.GetBool("otel.insecure")

	if otelInsecure {
		loggerOptions = append(loggerOptions, otelprovider.WithLogInsecure())
		tracerOptions = append(tracerOptions, otelprovider.WithTraceInsecure())
	}

	if strings.Contains(otelEndpoint, "4317") {
		loggerOptions = append(loggerOptions, otelprovider.WithGrpcLogEndpoint(otelEndpoint))
		tracerOptions = append(tracerOptions, otelprovider.WithGrpcTraceEndpoint(otelEndpoint))
	} else if strings.Contains(otelEndpoint, "4318") {
		loggerOptions = append(loggerOptions, otelprovider.WithHttpLogEndpoint(otelEndpoint))
		tracerOptions = append(tracerOptions, otelprovider.WithHttpTraceEndpoint(otelEndpoint))
	}

	logProvider := otelprovider.NewLogger(loggerOptions...)
	traceProvider := otelprovider.NewTracer(tracerOptions...)

	// Initialize Logging
	var zapLogger *zap.Logger
	var err error
	if debug {
		zapLogger, err = zap.NewDevelopment()
		gin.SetMode(gin.DebugMode)
	} else {
		zapLogger, err = zap.NewProduction()
		gin.SetMode(gin.ReleaseMode)
	}
	if err != nil {
		fmt.Printf("failed to initialize logger: %v", err)
		os.Exit(1)
	}

	// Replace zap global
	undoZapGlobals := zap.ReplaceGlobals(zapLogger)

	// Redirect stdlib log to zap
	undoStdLogRedirect := zap.RedirectStdLog(zapLogger)

	// Create otelLogger
	otelZapLogger := otelzap.New(zapLogger,
		otelzap.WithCaller(true),
		otelzap.WithMinLevel(zap.InfoLevel),
		otelzap.WithAnnotateLevel(zap.WarnLevel),
		otelzap.WithErrorStatusLevel(zap.ErrorLevel),
		otelzap.WithStackTrace(false),
		otelzap.WithLoggerProvider(logProvider),
	)

	// Replace global otelZap logger
	undoOtelZapGlobals := otelzap.ReplaceGlobals(otelZapLogger)

	return func() {
		ctx := context.Background()

		// Telemetry that didn't make it out is lost either way, and the
		// logger exports through the providers that just failed, so this goes
		// to stderr.
		err := errors.Join(
			traceProvider.ForceFlush(ctx),
			logProvider.ForceFlush(ctx),
			traceProvider.Shutdown(ctx),
			logProvider.Shutdown(ctx),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to flush and shut down the telemetry providers for %q: %v\n", otelEndpoint, err)
		}

		undoStdLogRedirect()
		undoOtelZapGlobals()
		undoZapGlobals()
	}
}
