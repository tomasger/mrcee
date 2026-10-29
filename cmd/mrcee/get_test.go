package main

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/urfave/cli/v3"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// outputTestServer exercises the real streaming protocol without job execution.
type outputTestServer struct {
	v1.UnimplementedJobServiceServer
	chunks   [][]byte
	fail     bool
	requests chan string
}

func (s *outputTestServer) StreamOutput(req *v1.StreamOutputRequest, stream v1.JobService_StreamOutputServer) error {
	s.requests <- req.GetJobId()
	for _, chunk := range s.chunks {
		if err := stream.Send(&v1.OutputChunk{Data: chunk}); err != nil {
			return err
		}
	}
	if s.fail {
		return grpcstatus.Error(codes.Internal, "test stream interrupted")
	}
	return nil
}

func TestGetOutput(t *testing.T) {
	type testCase struct {
		name       string
		chunks     [][]byte
		fail       bool
		wantOutput []byte
	}
	testCases := []testCase{
		{
			name: "empty_output",
		},
		{
			name: "multiple_binary_chunks",
			chunks: [][]byte{
				{'a', 0, 0xff},
				[]byte("100%\n"),
			},
			wantOutput: []byte{'a', 0, 0xff, '1', '0', '0', '%', '\n'},
		},
		{
			name: "failure_before_output",
			fail: true,
		},
		{
			name:       "failure_preserves_partial_output",
			chunks:     [][]byte{[]byte("partial\n")},
			fail:       true,
			wantOutput: []byte("partial\n"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan string, 1)
			client := newCommandTestClient(t, &outputTestServer{
				chunks:   tc.chunks,
				fail:     tc.fail,
				requests: requests,
			})

			var output bytes.Buffer
			cmd := getCmd()
			cmd.Writer = &output
			cmd.Action = func(ctx context.Context, c *cli.Command) error {
				return getOutput(ctx, c, client)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- cmd.Run(ctx, []string{"get", "test-job"}) }()
			var err error
			select {
			case err = <-done:
			case <-ctx.Done():
				t.Fatal("get did not return when the output stream ended")
			}
			if tc.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.True(t, bytes.Equal(tc.wantOutput, output.Bytes()), "job output must be preserved byte for byte")
			select {
			case jobID := <-requests:
				require.Equal(t, "test-job", jobID)
			default:
				t.Fatal("get did not request output from the service")
			}
		})
	}
}
