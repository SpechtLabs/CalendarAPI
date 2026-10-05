package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/apognu/gocal"
	"github.com/sierrasoftworks/humane-errors-go"
	"github.com/spechtlabs/go-otel-utils/otelzap"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
)

// AllCalendars is the calendar name that selects the events of every calendar.
const AllCalendars = "all"

const (
	utcZone = "UTC"

	// fetchTimeout bounds a single download of an iCal file.
	fetchTimeout = 30 * time.Second
)

// tzMapping maps the Windows time zone names Exchange and Outlook write into
// iCal files to IANA names.
var tzMapping = map[string]string{
	"Romance Standard Time":        "Europe/Brussels",
	"Pacific Standard Time":        "US/Pacific",
	"W. Europe Standard Time":      "Europe/Berlin",
	"E. Australia Standard Time":   "Australia/Brisbane",
	"GMT Standard Time":            "Europe/Dublin",
	"Eastern Standard Time":        "US/Eastern",
	"Greenwich Standard Time":      "Etc/GMT",
	"\tzone://Microsoft/Utc\"":     utcZone,
	"Central Europe Standard Time": "Europe/Berlin",
	"Central Standard Time":        "US/Central",
	"Customized Time Zone":         utcZone,
	"India Standard Time":          "Asia/Calcutta",
	"AUS Eastern Standard Time":    "Australia/Brisbane",
	utcZone:                        utcZone,
	"Israel Standard Time":         "Israel",
	"Singapore Standard Time":      "Singapore",
}

// ICalClient loads today's events from the configured iCal calendars, applies
// the configured rules to them, and serves them from a cache that
// FetchEvents refreshes. It also holds each calendar's custom status.
type ICalClient struct {
	tracer trace.Tracer
	clock  Clock
	http   *http.Client
	cache  *eventCache
	status *statusStore
}

// Calendar is one entry of the calendars config: an iCal file read from a
// path (From "file") or downloaded from a URL (From "url").
type Calendar struct {
	Name string `mapstructure:"name"`
	From string `mapstructure:"from"`
	Ical string `mapstructure:"ical"`
}

// eventCache holds the events FetchEvents loaded last.
type eventCache struct {
	resp *pb.CalendarResponse
	mu   sync.RWMutex
}

// statusStore holds the custom status of each calendar, by calendar name.
type statusStore struct {
	byCalendar map[string]*pb.CustomStatus
	mu         sync.RWMutex
}

// NewICalClient returns a client with an empty cache. It also installs the
// time zone mapping gocal uses for every parse.
func NewICalClient() *ICalClient {
	gocal.SetTZMapper(mapTimeZone)

	clock := WallClock{}
	return &ICalClient{
		tracer: otel.GetTracerProvider().Tracer("github.com/SpechtLabs/CalendarAPI/pkg/client"),
		clock:  clock,
		http:   &http.Client{Timeout: fetchTimeout},
		cache:  &eventCache{resp: &pb.CalendarResponse{LastUpdated: clock.Now().Unix()}},
		status: &statusStore{byCalendar: make(map[string]*pb.CustomStatus)},
	}
}

// Refresh fetches the events now and then every interval, until ctx is done.
func (e *ICalClient) Refresh(ctx context.Context, interval time.Duration) {
	tick, stop := e.clock.Tick(interval)
	defer stop()

	e.FetchEvents(ctx)
	for {
		select {
		case <-tick:
			e.FetchEvents(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// FetchEvents loads today's events from every configured calendar, applies
// the rules, and replaces the cache with the result. A calendar that fails to
// load contributes no events.
func (e *ICalClient) FetchEvents(ctx context.Context) {
	ctx, span := e.tracer.Start(ctx, "ICalClient.FetchEvents")
	defer span.End()

	start := e.clock.Now()

	// A config that fails to parse leaves no calendars, or no rules, which
	// keep no events; the wide event at the end reports why.
	var errs []error
	calendars, err := parseCalendars()
	if err != nil {
		errs = append(errs, err)
	}
	rules, err := parseRules()
	if err != nil {
		errs = append(errs, err)
	}

	// Each calendar loads into a slot of its own, so the goroutines share
	// nothing.
	type load struct {
		err    humane.Error
		events []*pb.CalendarEntry
	}
	loads := make([]load, len(calendars))

	var wg sync.WaitGroup
	for i, cal := range calendars {
		wg.Go(func() {
			events, err := e.loadEvents(ctx, cal, rules)
			loads[i] = load{err: err, events: events}
		})
	}
	wg.Wait()

	response := &pb.CalendarResponse{
		LastUpdated: e.clock.Now().Unix(),
		Entries:     make([]*pb.CalendarEntry, 0),
	}
	var failed []string
	for i, l := range loads {
		response.Entries = append(response.Entries, l.events...)
		if l.err != nil {
			failed = append(failed, calendars[i].Name)
			errs = append(errs, l.err)
		}
	}

	// Sort the events by start, then end, which the current-event lookup and
	// every client rely on.
	sort.Slice(response.Entries, func(i int, j int) bool {
		if response.Entries[i].Start == response.Entries[j].Start {
			return response.Entries[i].End < response.Entries[j].End
		}
		return response.Entries[i].Start < response.Entries[j].Start
	})

	e.cache.set(response)

	span.SetAttributes(
		attribute.Int("calendar.count", len(calendars)),
		attribute.Int("calendar.failed", len(failed)),
		attribute.Int("calendar.events", len(response.Entries)),
	)

	level := zap.InfoLevel
	if len(errs) > 0 {
		level = zap.ErrorLevel
		span.SetStatus(codes.Error, "the config or some calendars failed to load")
	}
	otelzap.Ctx(ctx).Logger().LogContext(ctx, level, "Refreshed calendars",
		zap.Int("calendars", len(calendars)),
		zap.Int("events", len(response.Entries)),
		zap.Strings("failed_calendars", failed),
		zap.Errors("errors", errs),
		zap.Duration("duration", e.clock.Now().Sub(start)),
	)
}

// GetEvents returns the cached events. The response is a copy, so the caller
// may filter its entries.
func (e *ICalClient) GetEvents(ctx context.Context) *pb.CalendarResponse {
	_, span := e.tracer.Start(ctx, "ICalClient.GetEvents")
	defer span.End()

	cached := e.cache.get()
	span.SetAttributes(attribute.Int("calendar.events", len(cached.Entries)))

	return &pb.CalendarResponse{
		LastUpdated:  cached.LastUpdated,
		CalendarName: cached.CalendarName,
		Entries:      slices.Clone(cached.Entries),
	}
}

// GetCurrentEvent returns the event of the calendar (or of AllCalendars) that
// is happening now, or nil when there is none. Of several, it returns the one
// that started last, preferring an important one among those that started at
// the same time.
func (e *ICalClient) GetCurrentEvent(ctx context.Context, calendar string) *pb.CalendarEntry {
	_, span := e.tracer.Start(ctx, "ICalClient.GetCurrentEvent")
	defer span.End()

	span.SetAttributes(attribute.String("calendar.name", calendar))

	cached := e.cache.get()

	var possibleCurrentEvents []*pb.CalendarEntry

	// Find all events happening right now
	now := e.clock.Now().Unix()
	for _, entry := range cached.Entries {
		if calendar != AllCalendars && entry.CalendarName != calendar {
			continue
		}

		if entry.Start < now && entry.End > now {
			possibleCurrentEvents = append(possibleCurrentEvents, entry)
		}
	}

	// If no events or only one event, return early
	switch len(possibleCurrentEvents) {
	case 0:
		return nil
	case 1:
		return possibleCurrentEvents[0]
	}

	// Find the event that starts or ends closest to now
	var closest *pb.CalendarEntry
	closestDelta := int64(math.MaxInt64)

	for _, entry := range possibleCurrentEvents {
		delta := now - entry.Start

		if delta == closestDelta && entry.Important && (closest == nil || !closest.Important) {
			closest = entry
		} else if delta < closestDelta {
			closest = entry
			closestDelta = delta
		}
	}

	return closest
}

// GetCustomStatus returns the calendar's custom status, or an empty one when
// none is set.
func (e *ICalClient) GetCustomStatus(ctx context.Context, req *pb.GetCustomStatusRequest) *pb.CustomStatus {
	_, span := e.tracer.Start(ctx, "ICalClient.GetCustomStatus")
	defer span.End()

	span.SetAttributes(attribute.String("calendar.name", req.GetCalendarName()))

	return e.status.get(req.GetCalendarName())
}

// SetCustomStatus sets the calendar's custom status; an empty status clears
// it.
func (e *ICalClient) SetCustomStatus(ctx context.Context, req *pb.SetCustomStatusRequest) {
	_, span := e.tracer.Start(ctx, "ICalClient.SetCustomStatus")
	defer span.End()

	span.SetAttributes(attribute.String("calendar.name", req.GetCalendarName()))

	e.status.set(req.GetCalendarName(), req.GetStatus())
}

// NewCalendarEntryFromGocalEvent converts a parsed iCal event into a calendar
// entry of the named calendar. Canceled and declined events, which Exchange
// marks in their summary, return nil.
func NewCalendarEntryFromGocalEvent(calName string, e gocal.Event) *pb.CalendarEntry {
	if strings.Contains(e.Summary, "Canceled") {
		return nil
	}

	if strings.Contains(e.Summary, "Declined") {
		return nil
	}

	busy := pb.BusyState_Free
	if val, ok := e.CustomAttributes["X-MICROSOFT-CDO-BUSYSTATUS"]; ok {
		switch val {
		case "BUSY":
			busy = pb.BusyState_Busy
		case "TENTATIVE":
			busy = pb.BusyState_Tentative

		case "FREE":
			busy = pb.BusyState_Free

		case "OOF":
			busy = pb.BusyState_OutOfOffice

		case "WORKINGELSEWHERE":
			busy = pb.BusyState_WorkingElsewhere
		}
	}

	allDay := false
	if val, ok := e.CustomAttributes["X-MICROSOFT-CDO-ALLDAYEVENT"]; ok {
		allDay = val == "TRUE"
	}

	start := e.Start.In(time.Local)
	end := e.End.In(time.Local)

	return &pb.CalendarEntry{
		Title:        e.Summary,
		Start:        start.Unix(),
		End:          end.Unix(),
		AllDay:       allDay,
		Busy:         busy,
		CalendarName: calName,
	}
}

func (c *eventCache) get() *pb.CalendarResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.resp
}

func (c *eventCache) set(resp *pb.CalendarResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resp = resp
}

func (s *statusStore) get(calendar string) *pb.CustomStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if val, ok := s.byCalendar[calendar]; ok && val != nil {
		return val
	}

	return &pb.CustomStatus{}
}

func (s *statusStore) set(calendar string, status *pb.CustomStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byCalendar[calendar] = status
}

// mapTimeZone is gocal's time zone mapper: it resolves the Windows names in
// tzMapping, and fails for anything else, which leaves the time zone to gocal.
func mapTimeZone(name string) (*time.Location, error) {
	tzid, ok := tzMapping[name]
	if !ok {
		return nil, humane.New(fmt.Sprintf("unknown time zone %q", name),
			"add the time zone's IANA name to tzMapping in pkg/client")
	}

	loc, err := time.LoadLocation(tzid)
	if err != nil {
		return nil, humane.Wrap(err, fmt.Sprintf("failed to load time zone %q", tzid),
			"check that the system has the IANA time zone database")
	}

	return loc, nil
}

func parseCalendars() ([]Calendar, humane.Error) {
	var calendars []Calendar
	if err := viper.UnmarshalKey("calendars", &calendars); err != nil {
		return nil, humane.Wrap(err, "failed to parse the calendars config",
			"check the calendars section of the config file against the documentation")
	}

	return calendars, nil
}

func safeIcalParse(ical io.Reader, now time.Time) (events []gocal.Event, err humane.Error) {
	// Filter to TODAY only
	today, _ := time.Parse(time.DateOnly, now.Format(time.DateOnly))
	eod := today.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	cal := gocal.NewParser(ical)
	start, end := today, eod
	cal.Start, cal.End = &start, &end

	// Protect against panics in gocal.Parse
	defer func() {
		if r := recover(); r != nil {
			err = humane.New(fmt.Sprintf("panic occurred while parsing iCal data: %v", r), "the iCal data might be malformed")
			events = nil
		}
	}()

	if err := cal.Parse(); err != nil {
		return nil, humane.Wrap(err, "unable to parse iCal file", "ensure the iCal file is valid and follows the iCal spec")
	}

	return cal.Events, nil
}

func (e *ICalClient) loadEvents(ctx context.Context, cal Calendar, rules []Rule) ([]*pb.CalendarEntry, humane.Error) {
	ctx, span := e.tracer.Start(ctx, "ICalClient.loadEvents")
	defer span.End()

	span.SetAttributes(
		attribute.String("calendar.name", cal.Name),
		attribute.String("calendar.from", cal.From),
		attribute.String("calendar.url", cal.Ical),
	)

	ical, err := e.getIcal(ctx, cal.From, cal.Ical)
	if err != nil {
		return nil, humane.Wrap(err, fmt.Sprintf("failed to load the iCal file of calendar %q", cal.Name),
			"check the calendar's 'from' and 'ical' settings")
	}

	calEvents, err := safeIcalParse(bytes.NewReader(ical), e.clock.Now())
	if err != nil {
		return nil, humane.Wrap(err, fmt.Sprintf("failed to parse the iCal file of calendar %q", cal.Name),
			"check that the file is a valid iCal calendar")
	}

	events := make([]*pb.CalendarEntry, 0)
	for _, evnt := range calEvents {
		event := NewCalendarEntryFromGocalEvent(cal.Name, evnt)
		if event != nil && applyRules(event, rules) {
			events = append(events, event)
		}
	}

	span.SetAttributes(attribute.Int("calendar.events", len(events)))

	return events, nil
}

// applyRules relabels the event with the first rule that matches it, and
// reports whether to keep the event: one that no rule matches, or whose
// first matching rule is a skip rule, is dropped.
func applyRules(event *pb.CalendarEntry, rules []Rule) bool {
	for i := range rules {
		if ok, skip := rules[i].Evaluate(event); ok {
			return !skip
		}
	}

	return false
}

func (e *ICalClient) getIcal(ctx context.Context, from string, url string) ([]byte, humane.Error) {
	switch from {
	case "file":
		return getIcalFromFile(url)
	case "url":
		return e.getIcalFromURL(ctx, url)
	default:
		return nil, humane.New("unsupported 'from' type", "The only supported values for 'from' are 'file' or 'url'")
	}
}

// getIcalFromFile reads a calendar file named in the config, which the
// operator controls.
func getIcalFromFile(path string) ([]byte, humane.Error) {
	file, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, humane.Wrap(err, "unable to read iCal File", "check if file path exists and is accessible")
	}

	return file, nil
}

func (e *ICalClient) getIcalFromURL(ctx context.Context, url string) ([]byte, humane.Error) {
	ctx, span := e.tracer.Start(ctx, "ICalClient.getIcalFromURL")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", http.MethodGet),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, humane.Wrap(err, fmt.Sprintf("failed creating request for %s", url), "verify if URL is valid and well-formed")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:60.0) Gecko/20100101 Firefox/81.0")

	resp, err := e.http.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, humane.Wrap(err, fmt.Sprintf("failed making request to %s", url), "verify if URL exists and is accessible")
	}
	defer func() { _ = resp.Body.Close() }()

	// Add error handling for non-2xx status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		statusErr := humane.New(fmt.Sprintf("%s answered with HTTP status %s", url, resp.Status),
			"check that the calendar's URL is current and publicly readable")
		span.SetStatus(codes.Error, "received non-2xx status code")
		span.RecordError(statusErr)
		return nil, statusErr
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, humane.Wrap(err, fmt.Sprintf("failed reading the response from %s", url), "check that the server sends the whole calendar")
	}

	return body, nil
}
