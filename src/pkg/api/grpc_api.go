package api

import (
	"context"
	"fmt"
	"net"

	"github.com/sierrasoftworks/humane-errors-go"
	"github.com/spechtlabs/go-otel-utils/otelzap"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/SpechtLabs/CalendarAPI/pkg/client"
	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
)

// GrpcApi serves the calendar service over gRPC.
type GrpcApi struct {
	pb.UnimplementedCalenderServiceServer
	calendars *client.ICalClient

	srv *grpc.Server
	lis net.Listener
}

// NewGrpcApiServer listens on the configured host and gRPC port, and returns
// the server that Serve runs on that listener.
func NewGrpcApiServer(calendars *client.ICalClient) (*GrpcApi, humane.Error) {
	addr := fmt.Sprintf("%s:%d", viper.GetString("server.host"), viper.GetInt("server.grpcPort"))

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, humane.Wrap(err, fmt.Sprintf("the gRPC API failed to listen on %s", addr),
			"check that server.host and server.grpcPort name a free port on this host")
	}

	// Create a server with the OpenTelemetry interceptor
	e := &GrpcApi{
		calendars: calendars,
		srv:       grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler())),
		lis:       lis,
	}

	pb.RegisterCalenderServiceServer(e.srv, e)

	return e, nil
}

// GetCalendar returns today's events of the requested calendar, or of every
// calendar.
func (e *GrpcApi) GetCalendar(ctx context.Context, req *pb.CalendarRequest) (*pb.CalendarResponse, error) {
	events := e.calendars.GetEvents(ctx)

	if req.CalendarName == "" || req.CalendarName == "*" {
		req.CalendarName = client.AllCalendars
	}

	events.CalendarName = req.CalendarName

	// if a specific calendar is requested, we must filter the entries down to the desired calendars
	if req.CalendarName != client.AllCalendars {
		var responseEvents []*pb.CalendarEntry
		for _, event := range events.Entries {
			if event.CalendarName == req.CalendarName {
				responseEvents = append(responseEvents, event)
			}
		}
		events.Entries = responseEvents
	}

	return events, nil
}

// GetCurrentEvent returns the event happening now in the requested calendar,
// or an empty event when there is none.
func (e *GrpcApi) GetCurrentEvent(ctx context.Context, req *pb.CalendarRequest) (*pb.CalendarEntry, error) {
	if req.CalendarName == "" || req.CalendarName == "*" {
		req.CalendarName = client.AllCalendars
	}

	currentEvent := e.calendars.GetCurrentEvent(ctx, req.CalendarName)
	return currentEvent, nil
}

// RefreshCalendar reloads every calendar now.
func (e *GrpcApi) RefreshCalendar(ctx context.Context, _ *pb.CalendarRequest) (*pb.RefreshCalendarResponse, error) {
	e.calendars.FetchEvents(ctx)
	return &pb.RefreshCalendarResponse{}, nil
}

// GetCustomStatus returns the calendar's custom status.
func (e *GrpcApi) GetCustomStatus(ctx context.Context, req *pb.GetCustomStatusRequest) (*pb.CustomStatus, error) {
	return e.calendars.GetCustomStatus(ctx, req), nil
}

// SetCustomStatus sets the calendar's custom status and returns it.
func (e *GrpcApi) SetCustomStatus(ctx context.Context, req *pb.SetCustomStatusRequest) (*pb.CustomStatus, error) {
	e.calendars.SetCustomStatus(ctx, req)
	return e.calendars.GetCustomStatus(ctx, &pb.GetCustomStatusRequest{CalendarName: req.CalendarName}), nil
}

// ClearCustomStatus clears the calendar's custom status and returns the
// empty one.
func (e *GrpcApi) ClearCustomStatus(ctx context.Context, req *pb.ClearCustomStatusRequest) (*pb.CustomStatus, error) {
	e.calendars.SetCustomStatus(ctx, &pb.SetCustomStatusRequest{CalendarName: req.CalendarName, Status: &pb.CustomStatus{}})
	return e.calendars.GetCustomStatus(ctx, &pb.GetCustomStatusRequest{CalendarName: req.CalendarName}), nil
}

// Serve serves the gRPC API until ctx is done, and then stops gracefully:
// calls in flight finish first.
func (e *GrpcApi) Serve(ctx context.Context) humane.Error {
	otelzap.Ctx(ctx).Info("gRPC API listening", zap.String("addr", e.Addr()))

	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			e.GracefulStop()
		case <-stopped:
		}
	}()

	if err := e.srv.Serve(e.lis); err != nil {
		return humane.Wrap(err, "the gRPC API stopped serving", "check the log above for the cause")
	}

	return nil
}

// GracefulStop stops the gRPC API once the calls in flight finish. Serve
// calls it when its context ends.
func (e *GrpcApi) GracefulStop() {
	e.srv.GracefulStop()
}

// Addr returns the address the gRPC API listens on.
func (e *GrpcApi) Addr() string {
	return e.lis.Addr().String()
}
