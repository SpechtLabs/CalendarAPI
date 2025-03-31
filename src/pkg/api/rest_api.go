package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	ginprometheus "github.com/mcuadros/go-gin-prometheus"
	"github.com/spf13/viper"
	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/proto"

	"github.com/SpechtLabs/CalendarAPI/pkg/client"
	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
)

type RestApi struct {
	client *client.ICalClient
	zapLog *otelzap.Logger
	srv    *http.Server
}

func NewRestApiServer(zapLog *otelzap.Logger, client *client.ICalClient) *RestApi {
	e := &RestApi{
		zapLog: zapLog,
		client: client,
	}

	// Setup Gin router
	router := gin.New(func(e *gin.Engine) {})

	// Setup otelgin to expose Open Telemetry
	router.Use(otelgin.Middleware("conf_room_display"))

	// Setup ginzap to log everything correctly to zap
	router.Use(ginzap.GinzapWithConfig(zapLog, &ginzap.Config{
		UTC:        true,
		TimeFormat: time.RFC3339,
		Context: ginzap.Fn(func(c *gin.Context) []zapcore.Field {
			fields := []zapcore.Field{}
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
		}),
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
		Addr:    fmt.Sprintf("%s:%d", viper.GetString("server.host"), viper.GetInt("server.httpPort")),
		Handler: router,
	}

	return e
}

func (e *RestApi) ListenAndServe() error {
	e.zapLog.Info(fmt.Sprintf("REST API Server listening at %s", e.srv.Addr))
	return e.srv.ListenAndServe()
}

func (e *RestApi) RefreshCalendar(ct *gin.Context) {
	e.client.FetchEvents(ct.Request.Context())
}

func (e *RestApi) GetCalendar(ct *gin.Context) {
	events := e.client.GetEvents(ct.Request.Context())

	queryParams := ct.Request.URL.Query()

	calendar := queryParams.Get("calendar")
	if calendar == "" || calendar == "*" {
		calendar = "all"
	}

	events.CalendarName = calendar

	// if a specific calendar is requested, we must filter the entries down to the desired calendars
	if calendar != "all" {
		var responseEvents []*pb.CalendarEntry
		for _, event := range events.Entries {
			if event.CalendarName == calendar {
				responseEvents = append(responseEvents, event)
			}
		}
		events.Entries = responseEvents
	}

	switch ct.ContentType() {
	case "application/protobuf":
		ct.ProtoBuf(http.StatusOK, events)
	default:
		ct.JSON(http.StatusOK, events)
	}
}

func (e *RestApi) GetCurrentEvent(ct *gin.Context) {
	queryParams := ct.Request.URL.Query()
	calendar := queryParams.Get("calendar")
	if calendar == "" || calendar == "*" {
		calendar = "all"
	}

	currentEvent := e.client.GetCurrentEvent(ct.Request.Context(), calendar)

	status := http.StatusOK
	if currentEvent == nil {
		status = http.StatusGone
	}

	switch ct.ContentType() {
	case "application/protobuf":
		ct.ProtoBuf(status, currentEvent)
	default:
		ct.JSON(status, currentEvent)
	}
}

func (e *RestApi) GetCustomStatus(ct *gin.Context) {
	queryParams := ct.Request.URL.Query()
	if !queryParams.Has("calendar") || queryParams.Get("calendar") == "" {
		ct.AbortWithError(http.StatusBadRequest, fmt.Errorf("missing 'calendar' parameter in query parameters: %v", queryParams.Encode()))
		return
	}

	calendar := queryParams.Get("calendar")
	getStatusReq := &pb.GetCustomStatusRequest{CalendarName: calendar}
	customStatus := e.client.GetCustomStatus(ct.Request.Context(), getStatusReq)

	status := http.StatusOK
	if len(customStatus.Title) == 0 {
		status = http.StatusGone
	}

	switch ct.ContentType() {
	case "application/protobuf":
		ct.ProtoBuf(status, customStatus)
	default:
		ct.JSON(status, customStatus)
	}
}

func (e *RestApi) SetCustomStatus(ct *gin.Context) {
	var err error
	var body []byte
	var customStatusReq pb.SetCustomStatusRequest

	switch ct.ContentType() {
	case "application/protobuf":
		body, err = io.ReadAll(ct.Request.Body)
		if err != nil {
			ct.ProtoBuf(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
			return
		}

		if err = proto.Unmarshal(body, &customStatusReq); err != nil {
			ct.ProtoBuf(http.StatusBadRequest, gin.H{"error": "Failed to parse request body"})
			return
		}

	default:
		body, err = io.ReadAll(ct.Request.Body)
		if err != nil {
			ct.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
			return
		}

		if err = json.Unmarshal(body, &customStatusReq); err != nil {
			ct.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse request body"})
			return
		}
	}

	e.client.SetCustomStatus(ct.Request.Context(), &customStatusReq)
}

func (e *RestApi) UnsetCustomStatus(ct *gin.Context) {
	var err error
	var body []byte
	var customStatusReq pb.ClearCustomStatusRequest

	switch ct.ContentType() {
	case "application/protobuf":
		body, err = io.ReadAll(ct.Request.Body)
		if err != nil {
			ct.ProtoBuf(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
			return
		}

		if err = proto.Unmarshal(body, &customStatusReq); err != nil {
			ct.ProtoBuf(http.StatusBadRequest, gin.H{"error": "Failed to parse request body"})
			return
		}

	default:
		body, err = io.ReadAll(ct.Request.Body)
		if err != nil {
			ct.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
			return
		}

		if err = json.Unmarshal(body, &customStatusReq); err != nil {
			ct.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse request body"})
			return
		}
	}

	e.client.SetCustomStatus(ct.Request.Context(), &pb.SetCustomStatusRequest{CalendarName: customStatusReq.CalendarName, Status: &pb.CustomStatus{}})
}

func (e *RestApi) Addr() string {
	return e.srv.Addr
}
