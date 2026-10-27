package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/tomasger/mrcee/job"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func newTestClient(t *testing.T, opts ...grpc.ServerOption) (v1.JobServiceClient, *job.Registry) {
	t.Helper()
	registry := job.NewRegistry()
	listener := bufconn.Listen(1024 * 1024)
	server := NewServer("", registry, opts...)
	go server.grpcServer.Serve(listener)
	t.Cleanup(func() {
		server.grpcServer.Stop()
		listener.Close()
	})

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return v1.NewJobServiceClient(conn), registry
}

func TestGRPCJobExitStatus(t *testing.T) {
	testCases := []struct {
		name     string
		exitCode int
	}{
		{
			name:     "exit_code_0",
			exitCode: 0,
		},
		{
			name:     "exit_code_7",
			exitCode: 7,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, registry := newTestClient(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			started, err := client.StartJob(ctx, &v1.StartJobRequest{
				Executable: "/bin/sh",
				Args:       []string{"-c", fmt.Sprintf("exit %d", tc.exitCode)},
			})
			require.NoError(t, err)
			require.NotEmpty(t, started.GetJobId())
			t.Cleanup(func() { registry.StopJob(started.GetJobId()) })

			require.Eventually(t, func() bool {
				stored, err := registry.JobStatus(started.GetJobId())
				return err == nil && stored.State == job.StateExited
			}, 5*time.Second, 10*time.Millisecond)

			finished, err := client.GetJobStatus(ctx, &v1.GetJobStatusRequest{JobId: started.GetJobId()})
			require.NoError(t, err)
			require.Equal(t, started.GetJobId(), finished.GetJobId())
			require.Equal(t, v1.JobState_JOB_STATE_EXITED, finished.GetState())
			require.NotNil(t, finished.ExitCode)
			require.Equal(t, int32(tc.exitCode), *finished.ExitCode)
			require.Nil(t, finished.TerminationSignal)
			require.NotNil(t, finished.StartedAt)
			require.NotNil(t, finished.FinishedAt)
			stored, err := registry.JobStatus(started.GetJobId())
			require.NoError(t, err)
			require.Equal(t, stored.StartedAt.UTC(), finished.StartedAt.AsTime())
			require.Equal(t, stored.FinishedAt.UTC(), finished.FinishedAt.AsTime())
		})
	}
}

func TestGRPCStreamOutput(t *testing.T) {
	testCases := []struct {
		name           string
		command        string
		want           []byte
		delayedOutput  bool
		expectedChunks int
	}{
		{
			name:           "output_larger_than_buffer",
			command:        fmt.Sprintf("head -c %d /dev/zero", 3*BufferSize),
			want:           bytes.Repeat([]byte{0}, 3*BufferSize),
			expectedChunks: 3,
		},
		{
			name:           "delayed_output",
			command:        `echo -n first; while [ ! -f "$1" ]; do sleep 0.01; done; sleep 0.05; echo -n second`,
			want:           []byte("firstsecond"),
			delayedOutput:  true,
			expectedChunks: 2,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, registry := newTestClient(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			// continueFile will be used as a signal for "delayed_output"
			// case which waits until a file exists.
			continueFile := filepath.Join(t.TempDir(), "continue")
			started, err := client.StartJob(ctx, &v1.StartJobRequest{
				Executable: "/bin/sh",
				Args:       []string{"-c", tc.command, "sh", continueFile},
			})
			require.NoError(t, err)
			t.Cleanup(func() { registry.StopJob(started.GetJobId()) })

			stream, err := client.StreamOutput(ctx, &v1.StreamOutputRequest{JobId: started.GetJobId()})
			require.NoError(t, err)
			var output []byte
			chunkCount := 0
			for {
				chunk, err := stream.Recv()
				if err == io.EOF {
					break
				}
				require.NoError(t, err)
				require.NotEmpty(t, chunk.GetData())
				require.LessOrEqual(t, len(chunk.GetData()), BufferSize)
				output = append(output, chunk.GetData()...)
				chunkCount++
				if tc.delayedOutput && chunkCount == 1 {
					require.Equal(t, "first", string(chunk.GetData()))
					// create continueFile now that the first chunk of data has arrived
					require.NoError(t, os.WriteFile(continueFile, nil, 0o600))
				}
			}
			require.Equal(t, tc.want, output)
			require.Equal(t, tc.expectedChunks, chunkCount)
		})
	}
}

func TestGRPCStopJob(t *testing.T) {
	client, registry := newTestClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	started, err := client.StartJob(ctx, &v1.StartJobRequest{Executable: "/bin/sleep", Args: []string{"60"}})
	require.NoError(t, err)
	t.Cleanup(func() { registry.StopJob(started.GetJobId()) })

	running, err := client.GetJobStatus(ctx, &v1.GetJobStatusRequest{JobId: started.GetJobId()})
	require.NoError(t, err)
	require.Equal(t, v1.JobState_JOB_STATE_RUNNING, running.GetState())
	require.Nil(t, running.FinishedAt)
	require.Nil(t, running.ExitCode)
	require.Nil(t, running.TerminationSignal)

	stopped, err := client.StopJob(ctx, &v1.StopJobRequest{JobId: started.GetJobId()})
	require.NoError(t, err)
	require.Equal(t, started.GetJobId(), stopped.GetJobId())
	require.Equal(t, v1.JobState_JOB_STATE_STOPPED, stopped.GetState())
	require.NotNil(t, stopped.FinishedAt)
	require.Nil(t, stopped.ExitCode)
	require.NotNil(t, stopped.TerminationSignal)
	require.Equal(t, int32(syscall.SIGKILL), *stopped.TerminationSignal)

	_, err = client.StopJob(ctx, &v1.StopJobRequest{JobId: started.GetJobId()})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestGRPCRequestErrors(t *testing.T) {
	client, _ := newTestClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	testCases := []struct {
		name string
		code codes.Code
		call func() error
	}{
		{
			name: "empty_executable",
			code: codes.InvalidArgument,
			call: func() error {
				_, err := client.StartJob(ctx, &v1.StartJobRequest{})
				return err
			},
		},
		{
			name: "missing_executable",
			code: codes.InvalidArgument,
			call: func() error {
				_, err := client.StartJob(ctx, &v1.StartJobRequest{Executable: "/nonexistent/mrcee-command"})
				return err
			},
		},
		{
			name: "empty_status_id",
			code: codes.InvalidArgument,
			call: func() error {
				_, err := client.GetJobStatus(ctx, &v1.GetJobStatusRequest{})
				return err
			},
		},
		{
			name: "empty_stop_id",
			code: codes.InvalidArgument,
			call: func() error {
				_, err := client.StopJob(ctx, &v1.StopJobRequest{})
				return err
			},
		},
		{
			name: "missing_status",
			code: codes.NotFound,
			call: func() error {
				_, err := client.GetJobStatus(ctx, &v1.GetJobStatusRequest{JobId: "unknown"})
				return err
			},
		},
		{
			name: "missing_stop",
			code: codes.NotFound,
			call: func() error {
				_, err := client.StopJob(ctx, &v1.StopJobRequest{JobId: "unknown"})
				return err
			},
		},
		{
			name: "empty_stream_id",
			code: codes.InvalidArgument,
			call: func() error {
				stream, err := client.StreamOutput(ctx, &v1.StreamOutputRequest{})
				if err != nil {
					return err
				}
				_, err = stream.Recv()
				return err
			},
		},
		{
			name: "missing_stream",
			code: codes.NotFound,
			call: func() error {
				stream, err := client.StreamOutput(ctx, &v1.StreamOutputRequest{JobId: "unknown"})
				if err != nil {
					return err
				}
				_, err = stream.Recv()
				return err
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.code, status.Code(tc.call()))
		})
	}
}

func TestGRPCStreamCancellationKeepsJobRunning(t *testing.T) {
	finished := make(chan error, 1)
	client, registry := newTestClient(t, grpc.StreamInterceptor(
		func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			err := handler(srv, stream)
			finished <- err
			return err
		},
	))
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	started, err := client.StartJob(ctx, &v1.StartJobRequest{
		Executable: "/bin/sh",
		Args:       []string{"-c", "echo -n ready; exec sleep 60"},
	})
	require.NoError(t, err)
	t.Cleanup(func() { registry.StopJob(started.GetJobId()) })

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	stream, err := client.StreamOutput(streamCtx, &v1.StreamOutputRequest{JobId: started.GetJobId()})
	require.NoError(t, err)
	chunk, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, "ready", string(chunk.GetData()))
	cancelStream()
	_, err = stream.Recv()
	require.Equal(t, codes.Canceled, status.Code(err))

	// The handler must also return while the process is still running.
	select {
	case err := <-finished:
		require.Equal(t, codes.Canceled, status.Code(err))
	case <-ctx.Done():
		t.Fatal("output handler did not return after cancellation")
	}
	running, err := client.GetJobStatus(ctx, &v1.GetJobStatusRequest{JobId: started.GetJobId()})
	require.NoError(t, err)
	require.Equal(t, v1.JobState_JOB_STATE_RUNNING, running.GetState())
}
