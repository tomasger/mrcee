package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// newCommandTestClient exercises RPC transport without TLS or job execution.
// Client certificates and connection cleanup are covered by client_test.go.
func newCommandTestClient(t *testing.T, service v1.JobServiceServer) v1.JobServiceClient {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	v1.RegisterJobServiceServer(grpcServer, service)
	go grpcServer.Serve(listener)
	t.Cleanup(func() {
		grpcServer.Stop()
		listener.Close()
	})
	conn, err := grpc.NewClient("passthrough:///test",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return v1.NewJobServiceClient(conn)
}

func runCommandWithClient(t *testing.T, cmd *cli.Command, action func(context.Context, *cli.Command, v1.JobServiceClient) error, client v1.JobServiceClient, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	cmd.Writer = &output
	cmd.ErrWriter = io.Discard
	cmd.Action = func(ctx context.Context, c *cli.Command) error {
		return action(ctx, c, client)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err := cmd.Run(ctx, append([]string{cmd.Name}, args...))
	return output.String(), err
}

func TestJobCommandsRejectInvalidArguments(t *testing.T) {
	type commandCase struct {
		name string
		new  func() *cli.Command
	}
	commands := []commandCase{
		{name: "status", new: statusCmd},
		{name: "get", new: getCmd},
		{name: "stop", new: stopCmd},
	}
	type argumentCase struct {
		name string
		args []string
	}
	testCases := []argumentCase{
		{name: "missing_job_id"},
		{name: "extra_argument", args: []string{"job-id", "extra"}},
		{name: "multiple_extra_arguments", args: []string{"job-id", "extra", "more"}},
	}
	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					cmd := command.new()
					cmd.Writer = io.Discard
					cmd.ErrWriter = io.Discard
					called := false
					cmd.Action = func(context.Context, *cli.Command) error {
						called = true
						return nil
					}
					err := cmd.Run(t.Context(), append([]string{command.name}, tc.args...))
					require.Error(t, err)
					require.False(t, called, "invalid arguments must be rejected before the action")
				})
			}
		})
	}
}
