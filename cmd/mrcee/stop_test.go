package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type stopTestServer struct {
	v1.UnimplementedJobServiceServer
	requests chan string
}

func (s *stopTestServer) StopJob(_ context.Context, req *v1.StopJobRequest) (*v1.JobStatus, error) {
	s.requests <- req.GetJobId()
	switch req.GetJobId() {
	case "finished-job":
		return nil, grpcstatus.Error(codes.FailedPrecondition, "test job is already stopped")
	case "running-job":
		return &v1.JobStatus{
			JobId:             req.GetJobId(),
			State:             v1.JobState_JOB_STATE_STOPPED,
			TerminationSignal: proto.Int32(9),
		}, nil
	default:
		return nil, grpcstatus.Error(codes.NotFound, "test job does not exist")
	}
}

func TestStop(t *testing.T) {
	type testCase struct {
		name      string
		jobID     string
		wantError bool
	}
	testCases := []testCase{
		{name: "stop_running_job", jobID: "running-job"},
		{name: "stop_already_finished_job", jobID: "finished-job", wantError: true},
		{name: "stop_missing_job", jobID: "missing-job", wantError: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan string, 1)
			client := newCommandTestClient(t, &stopTestServer{requests: requests})
			output, err := runCommandWithClient(t, stopCmd(), stopJob, client, tc.jobID)
			if tc.wantError {
				require.Error(t, err)
				require.Empty(t, output, "a failed stop must not announce termination")
			} else {
				require.NoError(t, err)
				require.Contains(t, output, "running-job")
			}
			select {
			case jobID := <-requests:
				require.Equal(t, tc.jobID, jobID)
			default:
				t.Fatal("stop did not request job termination")
			}
		})
	}
}
