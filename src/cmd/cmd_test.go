package cmd

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/SpechtLabs/CalendarAPI/pkg/api"
	"github.com/SpechtLabs/CalendarAPI/pkg/client"
	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
)

// registerOnce registers the flags and commands for TestFlags.
var registerOnce sync.Once

func TestRenderCalendar(t *testing.T) {
	now := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.Local)
	at := func(h int) int64 { return time.Date(2026, time.October, 5, h, 0, 0, 0, time.Local).Unix() }
	calendar := &pb.CalendarResponse{
		CalendarName: client.AllCalendars,
		LastUpdated:  now.Unix(),
		Entries: []*pb.CalendarEntry{
			{Title: "Standup", CalendarName: "work", Start: at(9), End: at(10)},
			{Title: "Holiday", CalendarName: "home", AllDay: true, Start: at(0), End: at(23)},
			{Title: "Focus", CalendarName: "work", Start: at(13), End: at(14), Important: true, Message: "Do not disturb"},
			{Title: "Gym", CalendarName: "home", Start: at(14), End: at(15), Busy: pb.BusyState_Tentative},
			{Title: "Offsite", CalendarName: "work", Start: at(15), End: at(16), Busy: pb.BusyState_OutOfOffice},
			{Title: "Cafe", CalendarName: "work", Start: at(16), End: at(17), Busy: pb.BusyState_WorkingElsewhere},
			{Title: "Reading", CalendarName: "home", Start: at(17), End: at(18), Busy: pb.BusyState_Busy},
		},
	}

	tests := []struct {
		format string
		want   []string
	}{
		{format: "json", want: []string{`"title":"Standup"`, `"calendar_name":"all"`}},
		{format: "yaml", want: []string{"title: Standup", "calendarname: all"}},
		{format: "text", want: []string{
			" 1) Holiday (all day)",
			" 2) Standup: <9:00AM - 10:00AM>",
			"Focus: <1:00PM - 2:00PM> - Do not disturb",
			"[Tentative]Gym",
			"[OutOfOffice]Offsite",
			"[WorkingElsewhere]Cafe",
			"(home)",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			out, err := renderCalendar(calendar, tt.format, now)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
		})
	}

	if got := renderEntry(nil, 1, now, false, newTextStyles()); got != "" {
		t.Errorf("renderEntry(nil) = %q, want empty", got)
	}
}

func TestCommandsAgainstServer(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("server.host", "127.0.0.1")
	viper.Set("server.grpcPort", 0)

	server, herr := api.NewGrpcApiServer(client.NewICalClient())
	if herr != nil {
		t.Fatal(herr)
	}
	go func() { _ = server.Serve(t.Context()) }()

	pointAt(t, server.Addr())
	calendar = "work"

	commands := []struct {
		name string
		run  func() error
	}{
		{name: "clear calendar", run: func() error { return runCmd(clearCalendarCmd, nil) }},
		{name: "get calendar", run: func() error { return runCmd(getCalendarCmd, []string{"work"}) }},
		{name: "get every calendar", run: func() error { return runCmd(getCalendarCmd, nil) }},
		{name: "set status", run: func() error { return runCmd(setCustomStatusCmd, []string{"Lunch"}) }},
		{name: "get status", run: func() error { return runCmd(getCustomStatusCmd, nil) }},
		{name: "clear status", run: func() error { return runCmd(clearCustomStatusCmd, nil) }},
		{name: "get cleared status", run: func() error { return runCmd(getCustomStatusCmd, nil) }},
	}

	for _, c := range commands {
		if err := c.run(); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}

	// Without a server, every command says so.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	pointAt(t, lis.Addr().String())
	_ = lis.Close()

	for _, c := range commands {
		if err := c.run(); err == nil || !strings.Contains(err.Error(), "failed to talk to the gRPC API") {
			t.Errorf("%s without a server: %v, want a connection error", c.name, err)
		}
	}

	outFormat = "xml-is-not-a-format-but-text-is-the-default"
	t.Cleanup(func() { outFormat = "text" })
	pointAt(t, server.Addr())
	if err := runCmd(getCalendarCmd, nil); err != nil {
		t.Errorf("get calendar with an unknown format: %v", err)
	}
}

func TestServe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("server.host", "127.0.0.1")
	viper.Set("server.grpcPort", 0)
	viper.Set("server.httpPort", 0)
	viper.Set("server.refresh", "not a duration")
	debug = true
	t.Cleanup(func() { debug = false })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := serve(ctx); err != nil {
		t.Errorf("serve() = %v, want nil after the context ends", err)
	}

	// A port that's taken stops serve.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lis.Close() }()
	_, port, _ := net.SplitHostPort(lis.Addr().String())

	viper.Set("server.grpcPort", port)
	if err := serve(context.Background()); err == nil {
		t.Error("serve() with the gRPC port taken succeeded")
	}

	viper.Set("server.grpcPort", 0)
	viper.Set("server.httpPort", port)
	if err := serve(context.Background()); err == nil {
		t.Error("serve() with the HTTP port taken succeeded")
	}
}

func TestRefresher(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("server.refresh", "1h")

	if got := refreshInterval(context.Background()); got != time.Hour {
		t.Errorf("refreshInterval() = %v, want 1h", got)
	}

	r := &calendarRefresher{calendars: client.NewICalClient(), loop: &refreshLoop{}}
	ctx := context.Background()
	r.restart(ctx)
	r.onConfigChange(ctx, fsnotify.Event{Name: "config.yaml"})

	viper.Set("server.host", "elsewhere")
	r.onConfigChange(ctx, fsnotify.Event{Name: "config.yaml"})
	r.stop()
}

func TestInitConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Cleanup(func() { configFileName = "" })

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  host: example\n  grpcPort: 1234\n  httpPort: 5678\n  debug: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	configFileName = path
	if err := initConfig(); err != nil {
		t.Fatal(err)
	}
	if hostname != "example" || grpcPort != 1234 || restPort != 5678 || !debug {
		t.Errorf("config read as host %q, ports %d/%d, debug %v", hostname, grpcPort, restPort, debug)
	}

	undo := initO11y()
	undo()
	debug = false
	initO11y()()

	configFileName = filepath.Join(dir, "missing.yaml")
	if err := initConfig(); err == nil {
		t.Error("initConfig() read a missing file")
	}

	// Without --config, it searches; the temporary directory has no
	// config.yaml beyond the one above, which it doesn't search.
	configFileName = ""
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	if err := initConfig(); err == nil {
		t.Error("initConfig() found a config file in an empty directory")
	}
}

func TestFlags(t *testing.T) {
	// Flags can only be defined once, and -count runs this again.
	var err error
	registerOnce.Do(func() {
		err = addRootFlags()
		addVerbCommands()
		addCalendarCommands()
		addCustomStatusCommands()
		addServeCommand()
		addVersionCommand()
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bindFlag("server.nothing", "no-such-flag"); err == nil {
		t.Error("bindFlag() bound a flag that doesn't exist")
	}

	// version runs without a config file.
	rootCmd.SetArgs([]string{"version"})
	if err := rootCmd.ExecuteContext(t.Context()); err != nil {
		t.Errorf("version: %v", err)
	}

	if _, _, err := rootCmd.Find([]string{"get", "status"}); err != nil {
		t.Errorf("get status isn't registered: %v", err)
	}
}

// runCmd runs the command as cobra would, with a context.
func runCmd(c *cobra.Command, args []string) error {
	c.SetContext(context.Background())
	return c.RunE(c, args)
}

// pointAt points the CLI's --server and --grpcPort at addr.
func pointAt(t *testing.T, addr string) {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	hostname = host
	if grpcPort, err = strconv.Atoi(port); err != nil {
		t.Fatal(err)
	}
}
