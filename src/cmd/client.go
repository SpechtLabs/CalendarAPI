package cmd

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/sierrasoftworks/humane-errors-go"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/SpechtLabs/CalendarAPI/pkg/protos"
)

// withCalendarAPI connects to the server's gRPC API at --server and
// --grpcPort, calls fn with a client for it and a context that ends after
// timeout, and closes the connection.
func withCalendarAPI(ctx context.Context, timeout time.Duration, fn func(context.Context, pb.CalenderServiceClient) error) humane.Error {
	addr := net.JoinHostPort(hostname, strconv.Itoa(grpcPort))

	// Set up a connection to the server with OpenTelemetry instrumentation
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return humane.Wrap(err, fmt.Sprintf("failed to set up a gRPC client for %s", addr),
			"check that --server and --grpcPort form a valid address")
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if err := fn(ctx, pb.NewCalenderServiceClient(conn)); err != nil {
		return humane.Wrap(err, fmt.Sprintf("failed to talk to the gRPC API at %s", addr),
			"check that `calendarapi serve` runs there, and that --server and --grpcPort point at it")
	}

	return nil
}
