package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type statusTestServer struct {
	v1.UnimplementedJobServiceServer
	requests chan string
	job      *v1.JobStatus
}

func (s *statusTestServer) GetJobStatus(_ context.Context, req *v1.GetJobStatusRequest) (*v1.JobStatus, error) {
	s.requests <- req.GetJobId()
	if s.job == nil {
		return nil, grpcstatus.Error(codes.NotFound, "test job does not exist")
	}
	return s.job, nil
}

func TestFetchJobStatus(t *testing.T) {
	started := time.Date(2026, time.October, 28, 13, 0, 0, 0, time.UTC)
	finished := started.Add(15*time.Second)
	type testCase struct {
		name     string
		response *v1.JobStatus
		want     *JobStatus
	}
	testCases := []testCase{
		{
			name: "running_job_has_no_completion_fields",
			response: &v1.JobStatus{
				JobId:     "reported-job",
				State:     v1.JobState_JOB_STATE_RUNNING,
				StartedAt: timestamppb.New(started),
			},
			want: &JobStatus{
				ID:        "reported-job",
				State:     v1.JobState_JOB_STATE_RUNNING,
				StartedAt: started,
			},
		},
		{
			name: "exited_job_preserves_zero_exit_code",
			response: &v1.JobStatus{
				JobId:      "reported-job",
				State:      v1.JobState_JOB_STATE_EXITED,
				StartedAt:  timestamppb.New(started),
				FinishedAt: timestamppb.New(finished),
				ExitCode:   proto.Int32(0),
			},
			want: &JobStatus{
				ID:         "reported-job",
				State:      v1.JobState_JOB_STATE_EXITED,
				StartedAt:  started,
				FinishedAt: &finished,
				ExitCode:   proto.Int32(0),
			},
		},
		{
			name: "stopped_job_preserves_signal",
			response: &v1.JobStatus{
				JobId:             "reported-job",
				State:             v1.JobState_JOB_STATE_STOPPED,
				StartedAt:         timestamppb.New(started),
				FinishedAt:        timestamppb.New(finished),
				TerminationSignal: proto.Int32(9),
			},
			want: &JobStatus{
				ID:                "reported-job",
				State:             v1.JobState_JOB_STATE_STOPPED,
				StartedAt:         started,
				FinishedAt:        &finished,
				TerminationSignal: proto.Int32(9),
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan string, 1)
			client := newCommandTestClient(t, &statusTestServer{requests: requests, job: tc.response})
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			job, err := fetchJobStatus(ctx, client, "requested-job")
			require.NoError(t, err)
			require.Equal(t, tc.want, job)
			select {
			case jobID := <-requests:
				require.Equal(t, "requested-job", jobID)
			default:
				t.Fatal("status did not request the job")
			}
		})
	}
}

func TestStatus(t *testing.T) {
	type testCase struct {
		name string
		job  *v1.JobStatus
	}
	testCases := []testCase{
		{
			name: "prints_returned_job",
			job: &v1.JobStatus{
				JobId:     "reported-job",
				State:     v1.JobState_JOB_STATE_RUNNING,
				StartedAt: timestamppb.Now(),
			},
		},
		{
			name: "missing_job_has_no_status_output",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan string, 1)
			client := newCommandTestClient(t, &statusTestServer{requests: requests, job: tc.job})
			output, err := runCommandWithClient(t, statusCmd(), status, client, "requested-job")
			if tc.job == nil {
				require.Error(t, err)
				require.Empty(t, output)
			} else {
				require.NoError(t, err)
				require.Contains(t, output, "reported-job")
			}
		})
	}
}
