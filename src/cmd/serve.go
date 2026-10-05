package cmd

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/sierrasoftworks/humane-errors-go"
	"github.com/spechtlabs/go-otel-utils/otelzap"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/SpechtLabs/CalendarAPI/pkg/api"
	"github.com/SpechtLabs/CalendarAPI/pkg/client"
)

var serveCmd = &cobra.Command{
	Use:     "serve",
	Short:   "Serves the calendars over the REST and gRPC APIs",
	Example: "meetingepd serve",
	Args:    cobra.ExactArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		return serve(cmd.Context())
	},
}

// calendarRefresher reloads the calendars on the configured interval, and
// starts over when the config changes.
type calendarRefresher struct {
	calendars *client.ICalClient
	loop      *refreshLoop
}

// refreshLoop runs one refresh loop at a time.
type refreshLoop struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
}

func addServeCommand() {
	rootCmd.AddCommand(serveCmd)
}

// serve runs both APIs and the calendar refresh until SIGINT or SIGTERM, or
// until either API fails, and then shuts everything down.
func serve(ctx context.Context) humane.Error {
	ctx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	if debug {
		logConfigFile(ctx)
	}

	calendars := client.NewICalClient()

	grpcAPI, err := api.NewGrpcApiServer(calendars)
	if err != nil {
		return err
	}
	restAPI := api.NewRestApiServer(calendars)

	refresher := &calendarRefresher{calendars: calendars, loop: &refreshLoop{}}
	refresher.restart(ctx)
	defer refresher.stop()

	viper.OnConfigChange(func(e fsnotify.Event) {
		refresher.onConfigChange(ctx, e)
	})
	viper.WatchConfig()

	// Either API stopping, on a signal or a failure, stops the other.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errs := make([]humane.Error, 2)
	wg.Go(func() {
		defer cancel()
		errs[0] = restAPI.ListenAndServe(ctx)
	})
	wg.Go(func() {
		defer cancel()
		errs[1] = grpcAPI.Serve(ctx)
	})
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	return nil
}

// logConfigFile logs the config file serve runs with, for --debug.
func logConfigFile(ctx context.Context) {
	path := viper.GetViper().ConfigFileUsed()
	file, err := os.ReadFile(path) // #nosec G304 -- the config file the operator chose
	if err != nil {
		otelzap.Ctx(ctx).Warn("Failed to read the config file to log it", zap.String("config_path", path), zap.Error(err))
		return
	}

	otelzap.Ctx(ctx).Debug("Config file used", zap.String("config_path", path), zap.String("config_file", string(file)))
}

// refreshInterval reads server.refresh, and falls back to
// defaultCalendarRefresh when it isn't a duration.
func refreshInterval(ctx context.Context) time.Duration {
	refreshConfig := viper.GetString("server.refresh")
	refresh, err := time.ParseDuration(refreshConfig)
	if err != nil {
		otelzap.Ctx(ctx).Warn("server.refresh is not a duration; using the default refresh interval",
			zap.String("refresh", refreshConfig),
			zap.Duration("default_refresh", defaultCalendarRefresh),
			zap.Error(err),
		)
		return defaultCalendarRefresh
	}

	return refresh
}

// restart stops the refresh loop that runs, if any, and starts a new one with
// the configured interval, which fetches the events right away.
func (r *calendarRefresher) restart(ctx context.Context) {
	interval := refreshInterval(ctx)

	r.loop.start(ctx, func(ctx context.Context) {
		r.calendars.Refresh(ctx, interval)
	})
}

// stop stops the refresh loop and waits for a fetch in flight to finish.
func (r *calendarRefresher) stop() {
	r.loop.stop()
}

// start stops the loop that runs, if any, and runs a new one until ctx ends
// or the next start or stop.
func (l *refreshLoop) start(ctx context.Context, run func(context.Context)) {
	ctx, cancel := context.WithCancel(ctx)

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cancel != nil {
		l.cancel()
	}
	l.cancel = cancel
	l.wg.Go(func() { run(ctx) })
}

// stop stops the loop that runs, if any, and waits for every loop to return.
func (l *refreshLoop) stop() {
	l.mu.Lock()
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
	l.mu.Unlock()

	l.wg.Wait()
}

// onConfigChange reloads the calendars with the changed config. The APIs keep
// the host and ports they listen on; changing those takes a restart.
func (r *calendarRefresher) onConfigChange(ctx context.Context, e fsnotify.Event) {
	r.restart(ctx)

	fields := []zap.Field{
		zap.String("filename", e.Name),
		zap.String("new_host", viper.GetString("server.host")),
		zap.String("old_host", hostname),
		zap.Int("new_grpcPort", viper.GetInt("server.grpcPort")),
		zap.Int("old_grpcPort", grpcPort),
		zap.Int("new_restPort", viper.GetInt("server.httpPort")),
		zap.Int("old_restPort", restPort),
	}

	if hostname != viper.GetString("server.host") ||
		grpcPort != viper.GetInt("server.grpcPort") ||
		restPort != viper.GetInt("server.httpPort") {
		otelzap.Ctx(ctx).Logger().LogContext(ctx, zap.ErrorLevel, "Config file changed; reloaded the calendars, but the host and ports only change on a restart", fields...)
		return
	}

	otelzap.Ctx(ctx).Logger().LogContext(ctx, zap.InfoLevel, "Config file changed; reloaded the calendars", fields...)
}
