package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"google.golang.org/protobuf/proto"

	"github.com/SpechtLabs/CalendarAPI/pkg/client"
	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
)

// newCalendars returns a client that loaded two calendars, each with one
// event that lasts all of today (UTC), so it is the current event too.
func newCalendars(t *testing.T) *client.ICalClient {
	t.Helper()

	viper.Reset()
	t.Cleanup(viper.Reset)

	// The client loads the events of the local day; in UTC, that is the day
	// the event below lasts.
	local := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = local })

	dir := t.TempDir()
	today := time.Now().UTC().Format("20060102")
	cfg := make([]map[string]any, 0, 2)
	for _, name := range []string{"work", "home"} {
		ics := fmt.Sprintf("BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//CalendarAPI//test//EN\n"+
			"BEGIN:VEVENT\nUID:%[1]s\nDTSTAMP:%[2]sT000000Z\nDTSTART:%[2]sT000001Z\nDTEND:%[2]sT235958Z\nSUMMARY:%[1]s day\nEND:VEVENT\n"+
			"END:VCALENDAR\n", name, today)
		path := filepath.Join(dir, name+".ics")
		if err := os.WriteFile(path, []byte(ics), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg = append(cfg, map[string]any{"name": name, "from": "file", "ical": path})
	}
	viper.Set("calendars", cfg)
	viper.Set("rules", []map[string]any{{"name": "all", "key": "*", "contains": []string{"*"}}})
	viper.Set("server.host", "127.0.0.1")
	viper.Set("server.httpPort", 0)
	viper.Set("server.grpcPort", 0)

	calendars := client.NewICalClient()
	calendars.FetchEvents(context.Background())
	return calendars
}

func TestRestCalendar(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewRestApiServer(newCalendars(t))

	tests := []struct {
		name       string
		query      string
		wantTitles []string
	}{
		{name: "every calendar", query: "", wantTitles: []string{"work day", "home day"}},
		{name: "one calendar", query: "?calendar=work", wantTitles: []string{"work day"}},
		// A request for one calendar used to filter the cached events for
		// every request after it.
		{name: "every calendar again", query: "?calendar=*", wantTitles: []string{"work day", "home day"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(api, http.MethodGet, "/calendar"+tt.query, "", nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}

			var resp pb.CalendarResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			assertTitles(t, resp.Entries, tt.wantTitles)
		})
	}

	t.Run("protobuf", func(t *testing.T) {
		rec := serve(api, http.MethodGet, "/calendar?calendar=home", contentTypeProtobuf, nil)
		var resp pb.CalendarResponse
		if err := proto.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		assertTitles(t, resp.Entries, []string{"home day"})
	})

	t.Run("refresh", func(t *testing.T) {
		if rec := serve(api, http.MethodPut, "/calendar", "", nil); rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})
}

func TestRestCurrentEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewRestApiServer(newCalendars(t))

	tests := []struct {
		name        string
		query       string
		contentType string
		wantCode    int
		wantTitle   string
	}{
		{name: "one calendar", query: "?calendar=home", wantCode: http.StatusOK, wantTitle: "home day"},
		{name: "protobuf", query: "?calendar=work", contentType: contentTypeProtobuf, wantCode: http.StatusOK, wantTitle: "work day"},
		{name: "no event", query: "?calendar=gym", wantCode: http.StatusGone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(api, http.MethodGet, "/calendar/current"+tt.query, tt.contentType, nil)
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if tt.wantTitle == "" {
				return
			}

			var entry pb.CalendarEntry
			var err error
			if tt.contentType == contentTypeProtobuf {
				err = proto.Unmarshal(rec.Body.Bytes(), &entry)
			} else {
				err = json.Unmarshal(rec.Body.Bytes(), &entry)
			}
			if err != nil {
				t.Fatal(err)
			}
			if entry.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", entry.Title, tt.wantTitle)
			}
		})
	}

	t.Run("every calendar", func(t *testing.T) {
		if rec := serve(api, http.MethodGet, "/calendar/current", "", nil); rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})
}

func TestRestCustomStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewRestApiServer(newCalendars(t))

	set, err := proto.Marshal(&pb.SetCustomStatusRequest{CalendarName: "home", Status: &pb.CustomStatus{Title: "Napping"}})
	if err != nil {
		t.Fatal(err)
	}

	steps := []struct {
		name        string
		method      string
		path        string
		contentType string
		body        []byte
		wantCode    int
		wantTitle   string
	}{
		{name: "missing calendar", method: http.MethodGet, path: "/status", wantCode: http.StatusBadRequest},
		{name: "not set", method: http.MethodGet, path: "/status?calendar=work", wantCode: http.StatusGone},
		{name: "set as JSON", method: http.MethodPost, path: "/status", body: []byte(`{"calendar_name":"work","status":{"title":"Lunch"}}`), wantCode: http.StatusOK},
		{name: "get as JSON", method: http.MethodGet, path: "/status?calendar=work", wantCode: http.StatusOK, wantTitle: "Lunch"},
		{name: "set as protobuf", method: http.MethodPost, path: "/status", contentType: contentTypeProtobuf, body: set, wantCode: http.StatusOK},
		{name: "get as protobuf", method: http.MethodGet, path: "/status?calendar=home", contentType: contentTypeProtobuf, wantCode: http.StatusOK, wantTitle: "Napping"},
		{name: "unparsable JSON", method: http.MethodPost, path: "/status", body: []byte(`{`), wantCode: http.StatusBadRequest},
		{name: "unparsable protobuf", method: http.MethodDelete, path: "/status", contentType: contentTypeProtobuf, body: []byte{0xff}, wantCode: http.StatusBadRequest},
		{name: "clear", method: http.MethodDelete, path: "/status", body: []byte(`{"calendar_name":"work"}`), wantCode: http.StatusOK},
		{name: "cleared", method: http.MethodGet, path: "/status?calendar=work", wantCode: http.StatusGone},
	}

	for _, s := range steps {
		rec := serve(api, s.method, s.path, s.contentType, s.body)
		if rec.Code != s.wantCode {
			t.Fatalf("%s: status = %d, want %d: %s", s.name, rec.Code, s.wantCode, rec.Body)
		}
		if s.wantTitle == "" {
			continue
		}

		var status pb.CustomStatus
		if s.contentType == contentTypeProtobuf {
			err = proto.Unmarshal(rec.Body.Bytes(), &status)
		} else {
			err = json.Unmarshal(rec.Body.Bytes(), &status)
		}
		if err != nil {
			t.Fatal(err)
		}
		if status.Title != s.wantTitle {
			t.Errorf("%s: title = %q, want %q", s.name, status.Title, s.wantTitle)
		}
	}
}

func TestRestUnreadableBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewRestApiServer(newCalendars(t))

	req := httptest.NewRequest(http.MethodPost, "/status", io.NopCloser(failingReader{}))
	rec := httptest.NewRecorder()
	api.srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Failed to read request body") {
		t.Errorf("got %d %s, want 400 for an unreadable body", rec.Code, rec.Body)
	}
}

func TestRestListenAndServe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := NewRestApiServer(newCalendars(t))
	if api.Addr() != "127.0.0.1:0" {
		t.Errorf("Addr() = %q, want 127.0.0.1:0", api.Addr())
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- api.ListenAndServe(ctx) }()
	cancel()
	if err := <-done; err != nil {
		t.Errorf("ListenAndServe() after cancel = %v, want nil", err)
	}

	// A port that's taken fails. A server that shut down never serves
	// again, so this takes a new one.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lis.Close() }()
	_, port, _ := net.SplitHostPort(lis.Addr().String())
	viper.Set("server.httpPort", port)
	if err := NewRestApiServer(client.NewICalClient()).ListenAndServe(context.Background()); err == nil {
		t.Error("ListenAndServe() on a taken port succeeded")
	}
}

func TestGrpcApi(t *testing.T) {
	api, herr := NewGrpcApiServer(newCalendars(t))
	if herr != nil {
		t.Fatal(herr)
	}
	ctx := context.Background()

	t.Run("calendar", func(t *testing.T) {
		resp, err := api.GetCalendar(ctx, &pb.CalendarRequest{})
		if err != nil {
			t.Fatal(err)
		}
		assertTitles(t, resp.Entries, []string{"work day", "home day"})

		resp, err = api.GetCalendar(ctx, &pb.CalendarRequest{CalendarName: "home"})
		if err != nil {
			t.Fatal(err)
		}
		assertTitles(t, resp.Entries, []string{"home day"})
	})

	t.Run("current event", func(t *testing.T) {
		entry, err := api.GetCurrentEvent(ctx, &pb.CalendarRequest{CalendarName: "work"})
		if err != nil || entry.GetTitle() != "work day" {
			t.Errorf("GetCurrentEvent() = %v, %v, want work day", entry, err)
		}
		entry, err = api.GetCurrentEvent(ctx, &pb.CalendarRequest{CalendarName: "*"})
		if err != nil || entry == nil {
			t.Errorf("GetCurrentEvent(*) = %v, %v, want an event", entry, err)
		}
	})

	t.Run("refresh", func(t *testing.T) {
		if _, err := api.RefreshCalendar(ctx, &pb.CalendarRequest{}); err != nil {
			t.Error(err)
		}
	})

	t.Run("custom status", func(t *testing.T) {
		status, err := api.SetCustomStatus(ctx, &pb.SetCustomStatusRequest{CalendarName: "work", Status: &pb.CustomStatus{Title: "Lunch"}})
		if err != nil || status.GetTitle() != "Lunch" {
			t.Fatalf("SetCustomStatus() = %v, %v", status, err)
		}
		status, err = api.GetCustomStatus(ctx, &pb.GetCustomStatusRequest{CalendarName: "work"})
		if err != nil || status.GetTitle() != "Lunch" {
			t.Fatalf("GetCustomStatus() = %v, %v", status, err)
		}
		status, err = api.ClearCustomStatus(ctx, &pb.ClearCustomStatusRequest{CalendarName: "work"})
		if err != nil || status.GetTitle() != "" {
			t.Fatalf("ClearCustomStatus() = %v, %v", status, err)
		}
	})

	t.Run("serve until canceled", func(t *testing.T) {
		serveCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- api.Serve(serveCtx) }()
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Serve() after cancel = %v, want nil", err)
		}
	})

	t.Run("taken port", func(t *testing.T) {
		host, port, err := net.SplitHostPort(api.Addr())
		if err != nil {
			t.Fatal(err)
		}
		lis, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = lis.Close() }()
		_, taken, _ := net.SplitHostPort(lis.Addr().String())
		if taken == port {
			t.Skip("the OS reused the port")
		}
		viper.Set("server.grpcPort", taken)
		if _, err := NewGrpcApiServer(client.NewICalClient()); err == nil {
			t.Error("NewGrpcApiServer() on a taken port succeeded")
		}
	})
}

func serve(api *RestApi, method, path, contentType string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	api.srv.Handler.ServeHTTP(rec, req)
	return rec
}

func assertTitles(t *testing.T, entries []*pb.CalendarEntry, want []string) {
	t.Helper()
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Title)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") && !sameSet(got, want) {
		t.Errorf("titles = %v, want %v", got, want)
	}
}

// sameSet reports whether a and b hold the same titles in any order: events
// that start at the same time keep no fixed order.
func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
