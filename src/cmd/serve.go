package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SpechtLabs/CalendarAPI/pkg/api"
	"github.com/SpechtLabs/CalendarAPI/pkg/client"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.uber.org/zap"
)

func initCalendarRefresh(zapLog *otelzap.Logger, iCalClient *client.ICalClient) chan struct{} {
	refreshConfig := viper.GetString("server.refresh")
	refresh, err := time.ParseDuration(refreshConfig)
	if err != nil {
		zapLog.Sugar().Errorf("Failed to parse '%s' as time.Duration: %v. Failing back to default refresh duration (%s)",
			refreshConfig, err.Error(),
			defaultCalendarRefresh,
		)
		refresh = defaultCalendarRefresh
	}

	refreshTicker := time.NewTicker(refresh)
	quitRefreshTicker := make(chan struct{})
	go func() {
		// initial load
		iCalClient.FetchEvents(context.Background())

		for {
			select {
			case <-refreshTicker.C:
				iCalClient.FetchEvents(context.Background())
			case <-quitRefreshTicker:
				refreshTicker.Stop()
				return
			}
		}
	}()

	return quitRefreshTicker
}

func viperConfigChange(undo func(), zapLog *zap.Logger, otelZap *otelzap.Logger, iCalClient *client.ICalClient, quitRefreshTicker *chan struct{}) {
	viper.OnConfigChange(func(e fsnotify.Event) {
		otelzap.L().Sugar().Infow("Config file change detected. Reloading.", "filename", e.Name)
		iCalClient.FetchEvents(context.Background())

		// refresh logger
		zapLog.Sync()
		undo()
		undo, zapLog, otelZap = initTelemetry()

		// Refresh calendar watch timer
		close(*quitRefreshTicker)
		*quitRefreshTicker = initCalendarRefresh(otelZap, iCalClient)

		if hostname != viper.GetString("server.host") ||
			grpcPort != viper.GetInt("server.grpcPort") ||
			restPort != viper.GetInt("server.httpPort") {
			zapLog.Sugar().Errorw("Unable to change host or port at runtime!",
				"new_host", viper.GetString("server.host"),
				"old_host", hostname,
				"new_grpcPort", viper.GetInt("server.grpcPort"),
				"old_grpcPort", grpcPort,
				"new_restPort", viper.GetInt("server.httpPort"),
				"old_grpcPort", restPort,
			)
		}
	})
}

var serveCmd = &cobra.Command{
	Use:     "serve",
	Short:   "Shows version information",
	Example: "meetingepd version",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		undo, zapLog, otelZap := initTelemetry()
		defer zapLog.Sync()
		defer undo()

		if debug {
			file, err := os.ReadFile(viper.GetViper().ConfigFileUsed())
			if err != nil {
				panic(fmt.Errorf("fatal error reading config file: %w", err))
			}
			zapLog.Sugar().With("config_file", string(file)).Debug("Config file used")
		}

		iCalClient := client.NewICalClient(otelZap)

		quitRefreshTicker := initCalendarRefresh(otelZap, iCalClient)
		viperConfigChange(undo, zapLog, otelZap, iCalClient, &quitRefreshTicker)
		viper.WatchConfig()

		// Serve Rest-API
		go func() {
			restApiServer := api.NewRestApiServer(otelZap, iCalClient)
			if err := restApiServer.ListenAndServe(); err != nil {
				panic(err.Error())
			}
		}()

		// Serve gRPC-API
		go func() {
			gRpcApiServer := api.NewGrpcApiServer(otelZap, iCalClient)
			if err := gRpcApiServer.Serve(); err != nil {
				panic(err.Error())
			}
		}()

		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c

		// close timer
		close(quitRefreshTicker)
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
