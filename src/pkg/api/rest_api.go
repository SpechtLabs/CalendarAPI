package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	ginprometheus "github.com/mcuadros/go-gin-prometheus"
	"github.com/sierrasoftworks/humane-errors-go"
	"github.com/spechtlabs/go-otel-utils/otelzap"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/proto"

	"github.com/SpechtLabs/CalendarAPI/pkg/client"
	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
)

const (
	// contentTypeProtobuf is the content type a request sets to get, or to
	// send, protobuf instead of JSON.
	contentTypeProtobuf = "application/protobuf"

	// readHeaderTimeout bounds how long a client may take to send its
	// request headers.
	readHeaderTimeout = 10 * time.Second

	// shutdownTimeout bounds how long requests in flight may take to finish
	// once the server shuts down.
	shutdownTimeout = 5 * time.Second
)

// RestApi serves the calendar service over HTTP, as JSON or protobuf.
type RestApi struct {
	calendars *client.ICalClient
	srv       *http.Server
}

// NewRestApiServer returns the REST API for the calendars, on the configured
// host and HTTP port. ListenAndServe starts it.
func NewRestApiServer(calendars *client.ICalClient) *RestApi {
	e := &RestApi{
		calendars: calendars,
	}

	// Setup Gin router
	router := gin.New(func(e *gin.Engine) {})

	// Setup otelgin to expose Open Telemetry
	router.Use(otelgin.Middleware("conf_room_display"))

	// Setup ginzap to log everything correctly to zap
	router.Use(ginzap.GinzapWithConfig(otelzap.L(), &ginzap.Config{
		UTC:        true,
		TimeFormat: time.RFC3339,
		Context: func(c *gin.Context) []zapcore.Field {
			var fields []zapcore.Field
			// log request ID
			if requestID := c.Writer.Header().Get("X-Request-Id"); requestID != "" {
				fields = append(fields, zap.String("request_id", requestID))
			}

			// log trace and span ID
			if trace.SpanFromContext(c.Request.Context()).SpanContext().IsValid() {
				fields = append(fields, zap.String("trace_id", trace.SpanFromContext(c.Request.Context()).SpanContext().TraceID().String()))
				fields = append(fields, zap.String("span_id", trace.SpanFromContext(c.Request.Context()).SpanContext().SpanID().String()))
			}
			return fields
		},
	}))

	// Set-up Prometheus to expose prometheus metrics
	p := ginprometheus.NewPrometheus("conf_room_display")
	p.Use(router)

	router.GET("/calendar", e.GetCalendar)
	router.GET("/calendar/current", e.GetCurrentEvent)
	router.PUT("/calendar", e.RefreshCalendar)
	router.GET("/status", e.GetCustomStatus)
	router.POST("/status", e.SetCustomStatus)
	router.DELETE("/status", e.UnsetCustomStatus)

	// configure the HTTP Server
	e.srv = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", viper.GetString("server.host"), viper.GetInt("server.httpPort")),
		Handler:           router,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	return e
}

// ListenAndServe serves the REST API until ctx is done, and then shuts down
// gracefully: requests in flight get shutdownTimeout to finish.
func (e *RestApi) ListenAndServe(ctx context.Context) humane.Error {
	otelzap.Ctx(ctx).Info("REST API listening", zap.String("addr", e.srv.Addr))

	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
			defer cancel()
			_ = e.srv.Shutdown(shutdownCtx)
		case <-stopped:
		}
	}()

	if err := e.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return humane.Wrap(err, fmt.Sprintf("the REST API failed to serve on %s", e.srv.Addr),
			"check that server.host and server.httpPort name a free port on this host")
	}

	return nil
}

// RefreshCalendar reloads every calendar now.
func (e *RestApi) RefreshCalendar(ct *gin.Context) {
	e.calendars.FetchEvents(ct.Request.Context())
}

// GetCalendar answers with today's events of the calendar the query names,
// or of every calendar.
func (e *RestApi) GetCalendar(ct *gin.Context) {
	events := e.calendars.GetEvents(ct.Request.Context())

	queryParams := ct.Request.URL.Query()

	calendar := queryParams.Get("calendar")
	if calendar == "" || calendar == "*" {
		calendar = client.AllCalendars
	}

	events.CalendarName = calendar

	// if a specific calendar is requested, we must filter the entries down to the desired calendars
	if calendar != client.AllCalendars {
		var responseEvents []*pb.CalendarEntry
		for _, event := range events.Entries {
			if event.CalendarName == calendar {
				responseEvents = append(responseEvents, event)
			}
		}
		events.Entries = responseEvents
	}

	switch ct.ContentType() {
	case contentTypeProtobuf:
		ct.ProtoBuf(http.StatusOK, events)
	default:
		ct.JSON(http.StatusOK, events)
	}
}

// GetCurrentEvent answers with the event happening now in the calendar the
// query names, or 410 Gone when there is none.
func (e *RestApi) GetCurrentEvent(ct *gin.Context) {
	queryParams := ct.Request.URL.Query()
	calendar := queryParams.Get("calendar")
	if calendar == "" || calendar == "*" {
		calendar = client.AllCalendars
	}

	currentEvent := e.calendars.GetCurrentEvent(ct.Request.Context(), calendar)

	status := http.StatusOK
	if currentEvent == nil {
		status = http.StatusGone
	}

	switch ct.ContentType() {
	case contentTypeProtobuf:
		ct.ProtoBuf(status, currentEvent)
	default:
		ct.JSON(status, currentEvent)
	}
}

// GetCustomStatus answers with the custom status of the calendar the query
// names, or 410 Gone when none is set.
func (e *RestApi) GetCustomStatus(ct *gin.Context) {
	queryParams := ct.Request.URL.Query()
	if !queryParams.Has("calendar") || queryParams.Get("calendar") == "" {
		_ = ct.AbortWithError(http.StatusBadRequest, humane.New(
			fmt.Sprintf("missing 'calendar' parameter in query parameters: %v", queryParams.Encode()),
			"name the calendar in the query, e.g. /status?calendar=office"))
		return
	}

	calendar := queryParams.Get("calendar")
	getStatusReq := &pb.GetCustomStatusRequest{CalendarName: calendar}
	customStatus := e.calendars.GetCustomStatus(ct.Request.Context(), getStatusReq)

	status := http.StatusOK
	if len(customStatus.Title) == 0 {
		status = http.StatusGone
	}

	switch ct.ContentType() {
	case contentTypeProtobuf:
		ct.ProtoBuf(status, customStatus)
	default:
		ct.JSON(status, customStatus)
	}
}

// SetCustomStatus sets the custom status the request body describes.
func (e *RestApi) SetCustomStatus(ct *gin.Context) {
	var customStatusReq pb.SetCustomStatusRequest
	if !readRequest(ct, &customStatusReq) {
		return
	}

	e.calendars.SetCustomStatus(ct.Request.Context(), &customStatusReq)
}

// UnsetCustomStatus clears the custom status of the calendar the request
// body names.
func (e *RestApi) UnsetCustomStatus(ct *gin.Context) {
	var customStatusReq pb.ClearCustomStatusRequest
	if !readRequest(ct, &customStatusReq) {
		return
	}

	e.calendars.SetCustomStatus(ct.Request.Context(), &pb.SetCustomStatusRequest{CalendarName: customStatusReq.CalendarName, Status: &pb.CustomStatus{}})
}

// Addr returns the address the REST API listens on.
func (e *RestApi) Addr() string {
	return e.srv.Addr
}

// readRequest decodes the request body into msg, as protobuf when the request
// says so and as JSON otherwise. When it can't, it answers 400 with the
// reason as JSON, and returns false.
func readRequest(ct *gin.Context, msg proto.Message) bool {
	body, err := io.ReadAll(ct.Request.Body)
	if err != nil {
		ct.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return false
	}

	if ct.ContentType() == contentTypeProtobuf {
		err = proto.Unmarshal(body, msg)
	} else {
		err = json.Unmarshal(body, msg)
	}
	if err != nil {
		ct.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Failed to parse request body"})
		return false
	}

	return true
}
