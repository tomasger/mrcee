package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

type startTestServer struct {
	v1.UnimplementedJobServiceServer
	requests chan *v1.StartJobRequest
	reject   bool
}

func (s *startTestServer) StartJob(_ context.Context, req *v1.StartJobRequest) (*v1.StartJobResponse, error) {
	s.requests <- req
	if s.reject {
		return nil, grpcstatus.Error(codes.PermissionDenied, "test role cannot start jobs")
	}
	return &v1.StartJobResponse{JobId: "created-job"}, nil
}

func TestStart(t *testing.T) {
	type testCase struct {
		name           string
		args           []string
		wantExecutable string
		wantArgs       []string
		reject         bool
	}
	testCases := []testCase{
		{
			name:           "executable_without_arguments",
			args:           []string{"--", "echo"},
			wantExecutable: "echo",
		},
		{
			name:           "arguments_preserve_spaces_flags_and_shell_characters",
			args:           []string{"--", "echo", "-n", "hello world", "|", "$HOME", ""},
			wantExecutable: "echo",
			wantArgs:       []string{"-n", "hello world", "|", "$HOME", ""},
		},
		{
			name:           "explicit_shell_command",
			args:           []string{"--", "sh", "-c", "echo hello | wc -c"},
			wantExecutable: "sh",
			wantArgs:       []string{"-c", "echo hello | wc -c"},
		},
		{
			name:           "rejected_start_has_no_success_output",
			args:           []string{"--", "echo", "hello"},
			wantExecutable: "echo",
			wantArgs:       []string{"hello"},
			reject:         true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan *v1.StartJobRequest, 1)
			client := newCommandTestClient(t, &startTestServer{requests: requests, reject: tc.reject})
			output, err := runCommandWithClient(t, startCmd(), start, client, tc.args...)
			if tc.reject {
				require.Error(t, err)
				require.Empty(t, output, "a rejected start must not announce a job")
			} else {
				require.NoError(t, err)
				require.Contains(t, output, "created-job", "print the ID returned by the service")
			}
			select {
			case req := <-requests:
				require.Equal(t, tc.wantExecutable, req.GetExecutable())
				require.Equal(t, tc.wantArgs, req.GetArgs())
			default:
				t.Fatal("start did not submit a job")
			}
		})
	}
}

func TestStartWithoutExecutableDoesNotSubmitJob(t *testing.T) {
	requests := make(chan *v1.StartJobRequest, 1)
	client := newCommandTestClient(t, &startTestServer{requests: requests})
	output, err := runCommandWithClient(t, startCmd(), start, client)
	require.Error(t, err)
	require.Empty(t, output)
	select {
	case <-requests:
		t.Fatal("a missing executable must be rejected before submitting a job")
	default:
	}
}
