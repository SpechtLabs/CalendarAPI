package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apognu/gocal"
	"github.com/spf13/viper"

	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
)

// testNow is 09:15 UTC on the day the test calendars' events happen.
var testNow = time.Date(2026, time.October, 5, 9, 15, 0, 0, time.UTC)

const workCalendar = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//CalendarAPI//test//EN
BEGIN:VEVENT
UID:standup
DTSTAMP:20261001T000000Z
DTSTART:20261005T090000Z
DTEND:20261005T093000Z
SUMMARY:Standup
X-MICROSOFT-CDO-BUSYSTATUS:BUSY
END:VEVENT
BEGIN:VEVENT
UID:review
DTSTAMP:20261001T000000Z
DTSTART:20261005T100000Z
DTEND:20261005T110000Z
SUMMARY:Canceled: Review
END:VEVENT
BEGIN:VEVENT
UID:tomorrow
DTSTAMP:20261001T000000Z
DTSTART:20261006T090000Z
DTEND:20261006T100000Z
SUMMARY:Planning
END:VEVENT
END:VCALENDAR
`

const homeCalendar = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//CalendarAPI//test//EN
BEGIN:VEVENT
UID:focus
DTSTAMP:20261001T000000Z
DTSTART:20261005T080000Z
DTEND:20261005T100000Z
SUMMARY:Focus time
X-MICROSOFT-CDO-BUSYSTATUS:OOF
END:VEVENT
END:VCALENDAR
`

// fakeClock stands still at now, and ticks when the test sends on tick.
type fakeClock struct {
	now  time.Time
	tick chan time.Time
}

func (c fakeClock) Now() time.Time { return c.now }

func (c fakeClock) Tick(time.Duration) (<-chan time.Time, func()) { return c.tick, func() {} }

func TestFetchEvents(t *testing.T) {
	dir := t.TempDir()
	workFile := filepath.Join(dir, "work.ics")
	if err := os.WriteFile(workFile, []byte(workCalendar), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/home.ics" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(homeCalendar))
	}))
	defer srv.Close()

	setConfig(t, map[string]any{
		"calendars": []map[string]any{
			{"name": "work", "from": "file", "ical": workFile},
			{"name": "home", "from": "url", "ical": srv.URL + "/home.ics"},
			{"name": "gone", "from": "url", "ical": srv.URL + "/gone.ics"},
			{"name": "missing", "from": "file", "ical": filepath.Join(dir, "missing.ics")},
			{"name": "odd", "from": "carrier-pigeon", "ical": "coo"},
		},
		"rules": []map[string]any{
			{"name": "focus", "key": "title", "contains": []string{"focus"}, "message": "Do not disturb", "important": true},
			{"name": "everything", "key": "*", "contains": []string{"*"}},
		},
	})

	c := newTestClient()
	c.FetchEvents(context.Background())

	got := c.GetEvents(context.Background())
	if got.LastUpdated != testNow.Unix() {
		t.Errorf("LastUpdated = %d, want %d", got.LastUpdated, testNow.Unix())
	}

	want := []*pb.CalendarEntry{
		{Title: "Focus time", CalendarName: "home", Start: hour(8, 0), End: hour(10, 0), Busy: pb.BusyState_OutOfOffice, Message: "Do not disturb", Important: true},
		{Title: "Standup", CalendarName: "work", Start: hour(9, 0), End: hour(9, 30), Busy: pb.BusyState_Busy},
	}
	assertEntries(t, got.Entries, want)
}

func TestFetchEventsWithoutConfig(t *testing.T) {
	setConfig(t, map[string]any{"calendars": "not a list", "rules": "not a list"})

	c := newTestClient()
	c.FetchEvents(context.Background())

	if got := c.GetEvents(context.Background()); len(got.Entries) != 0 {
		t.Errorf("got %d entries from a broken config, want none", len(got.Entries))
	}
}

func TestRefresh(t *testing.T) {
	setConfig(t, map[string]any{})

	c := newTestClient()
	clock := fakeClock{now: testNow, tick: make(chan time.Time)}
	c.clock = clock

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Refresh(ctx, time.Minute)
		close(done)
	}()

	// The send returns once Refresh is waiting for ticks, so the initial
	// fetch is done.
	clock.tick <- testNow
	cancel()
	<-done

	if got := c.GetEvents(context.Background()).LastUpdated; got != testNow.Unix() {
		t.Errorf("LastUpdated = %d, want %d", got, testNow.Unix())
	}
}

func TestGetEventsReturnsACopy(t *testing.T) {
	c := newTestClient()
	c.cache.set(&pb.CalendarResponse{Entries: []*pb.CalendarEntry{
		{Title: "a", CalendarName: "work"},
		{Title: "b", CalendarName: "home"},
	}})

	// What the APIs do to answer for one calendar.
	events := c.GetEvents(context.Background())
	events.CalendarName = "work"
	events.Entries = events.Entries[:1]

	if got := c.GetEvents(context.Background()); len(got.Entries) != 2 || got.CalendarName != "" {
		t.Errorf("the cache changed with the response: %d entries for calendar %q", len(got.Entries), got.CalendarName)
	}
}

func TestGetCurrentEvent(t *testing.T) {
	entries := []*pb.CalendarEntry{
		{Title: "early", CalendarName: "work", Start: hour(8, 0), End: hour(12, 0)},
		{Title: "standup", CalendarName: "work", Start: hour(9, 0), End: hour(9, 30)},
		{Title: "focus", CalendarName: "home", Start: hour(9, 0), End: hour(10, 0), Important: true},
		{Title: "lunch", CalendarName: "home", Start: hour(12, 0), End: hour(13, 0)},
	}

	tests := []struct {
		name      string
		calendar  string
		entries   []*pb.CalendarEntry
		wantTitle string
	}{
		{name: "latest start wins, important among equals", calendar: AllCalendars, entries: entries, wantTitle: "focus"},
		{name: "one calendar", calendar: "work", entries: entries, wantTitle: "standup"},
		{name: "single candidate", calendar: "work", entries: entries[:1], wantTitle: "early"},
		{name: "nothing now", calendar: "home", entries: entries[3:]},
		{name: "unknown calendar", calendar: "gym", entries: entries},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient()
			c.cache.set(&pb.CalendarResponse{Entries: tt.entries})

			got := c.GetCurrentEvent(context.Background(), tt.calendar)
			if got.GetTitle() != tt.wantTitle {
				t.Errorf("GetCurrentEvent() = %q, want %q", got.GetTitle(), tt.wantTitle)
			}
		})
	}
}

func TestCustomStatus(t *testing.T) {
	c := newTestClient()
	ctx := context.Background()

	if got := c.GetCustomStatus(ctx, &pb.GetCustomStatusRequest{CalendarName: "work"}); got.GetTitle() != "" {
		t.Fatalf("status before setting one = %q, want empty", got.GetTitle())
	}

	c.SetCustomStatus(ctx, &pb.SetCustomStatusRequest{CalendarName: "work", Status: &pb.CustomStatus{Title: "Lunch"}})
	if got := c.GetCustomStatus(ctx, &pb.GetCustomStatusRequest{CalendarName: "work"}); got.GetTitle() != "Lunch" {
		t.Errorf("status = %q, want Lunch", got.GetTitle())
	}
	if got := c.GetCustomStatus(ctx, &pb.GetCustomStatusRequest{CalendarName: "home"}); got.GetTitle() != "" {
		t.Errorf("another calendar's status = %q, want empty", got.GetTitle())
	}

	c.SetCustomStatus(ctx, &pb.SetCustomStatusRequest{CalendarName: "work"})
	if got := c.GetCustomStatus(ctx, &pb.GetCustomStatusRequest{CalendarName: "work"}); got == nil || got.GetTitle() != "" {
		t.Errorf("status after clearing = %v, want an empty status", got)
	}
}

func TestNewCalendarEntryFromGocalEvent(t *testing.T) {
	start := time.Unix(hour(9, 0), 0)
	end := time.Unix(hour(10, 0), 0)
	event := func(summary string, attrs map[string]string) gocal.Event {
		return gocal.Event{Summary: summary, Start: &start, End: &end, CustomAttributes: attrs}
	}

	tests := []struct {
		name  string
		event gocal.Event
		want  *pb.CalendarEntry
	}{
		{
			name:  "plain event is free",
			event: event("Standup", nil),
			want:  &pb.CalendarEntry{Title: "Standup", Busy: pb.BusyState_Free},
		},
		{name: "busy", event: event("a", map[string]string{"X-MICROSOFT-CDO-BUSYSTATUS": "BUSY"}), want: &pb.CalendarEntry{Title: "a", Busy: pb.BusyState_Busy}},
		{name: "tentative", event: event("a", map[string]string{"X-MICROSOFT-CDO-BUSYSTATUS": "TENTATIVE"}), want: &pb.CalendarEntry{Title: "a", Busy: pb.BusyState_Tentative}},
		{name: "free", event: event("a", map[string]string{"X-MICROSOFT-CDO-BUSYSTATUS": "FREE"}), want: &pb.CalendarEntry{Title: "a", Busy: pb.BusyState_Free}},
		{name: "out of office", event: event("a", map[string]string{"X-MICROSOFT-CDO-BUSYSTATUS": "OOF"}), want: &pb.CalendarEntry{Title: "a", Busy: pb.BusyState_OutOfOffice}},
		{name: "working elsewhere", event: event("a", map[string]string{"X-MICROSOFT-CDO-BUSYSTATUS": "WORKINGELSEWHERE"}), want: &pb.CalendarEntry{Title: "a", Busy: pb.BusyState_WorkingElsewhere}},
		{name: "all day", event: event("a", map[string]string{"X-MICROSOFT-CDO-ALLDAYEVENT": "TRUE"}), want: &pb.CalendarEntry{Title: "a", AllDay: true}},
		{name: "canceled", event: event("Canceled: Review", nil)},
		{name: "declined", event: event("Declined: Review", nil)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewCalendarEntryFromGocalEvent("work", tt.event)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("got %v, want nil", got)
				}
				return
			}

			tt.want.CalendarName = "work"
			tt.want.Start, tt.want.End = start.Unix(), end.Unix()
			assertEntries(t, []*pb.CalendarEntry{got}, []*pb.CalendarEntry{tt.want})
		})
	}
}

func TestMapTimeZone(t *testing.T) {
	tests := []struct {
		name    string
		zone    string
		want    string
		wantErr bool
	}{
		{name: "windows name", zone: "W. Europe Standard Time", want: "Europe/Berlin"},
		{name: "utc", zone: utcZone, want: utcZone},
		{name: "unknown", zone: "Mars Standard Time", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := mapTimeZone(tt.zone)
			if (err != nil) != tt.wantErr {
				t.Fatalf("mapTimeZone(%q) error = %v, want error %v", tt.zone, err, tt.wantErr)
			}
			if err == nil && loc.String() != tt.want {
				t.Errorf("mapTimeZone(%q) = %s, want %s", tt.zone, loc, tt.want)
			}
		})
	}
}

func TestGetIcalFromURLFailures(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "malformed url", url: "http://bad host/", want: "failed creating request"},
		{name: "unreachable server", url: closed.URL, want: "failed making request"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newTestClient().getIcalFromURL(context.Background(), tt.url)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("getIcalFromURL(%q) error = %v, want one containing %q", tt.url, err, tt.want)
			}
		})
	}
}

func TestSafeIcalParseRejectsMalformedInput(t *testing.T) {
	_, err := safeIcalParse(strings.NewReader("BEGIN:VCALENDAR\nBEGIN:VEVENT\nUID:bad\nDTSTAMP:20261001T000000Z\nDTSTART:yesterday\nEND:VEVENT\nEND:VCALENDAR\n"), testNow)
	if err == nil {
		t.Fatal("safeIcalParse() accepted an event with an invalid start")
	}
}

func newTestClient() *ICalClient {
	c := NewICalClient()
	c.clock = fakeClock{now: testNow}
	return c
}

// setConfig replaces viper's config with values for the test.
func setConfig(t *testing.T, values map[string]any) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	for k, v := range values {
		viper.Set(k, v)
	}
}

// hour returns the Unix time of hh:mm UTC on testNow's day.
func hour(hh, mm int) int64 {
	return time.Date(testNow.Year(), testNow.Month(), testNow.Day(), hh, mm, 0, 0, time.UTC).Unix()
}

func assertEntries(t *testing.T, got, want []*pb.CalendarEntry) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d entries %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Title != w.Title || g.CalendarName != w.CalendarName || g.Start != w.Start || g.End != w.End ||
			g.Busy != w.Busy || g.AllDay != w.AllDay || g.Message != w.Message || g.Important != w.Important {
			t.Errorf("entry %d = %v, want %v", i, g, w)
		}
	}
}
