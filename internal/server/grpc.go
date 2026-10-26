package server

import (
	"bytes"
	"context"
	"errors"
	"io"

	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/tomasger/mrcee/job"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// BufferSize is the size of the buffer used in stream output
const BufferSize = 32 * 1024

// GRPCServer exposes the job registry through the JobService API.
type GRPCServer struct {
	v1.UnimplementedJobServiceServer
	registry *job.Registry
}

func NewGRPCServer(registry *job.Registry) *GRPCServer {
	return &GRPCServer{registry: registry}
}

func (s *GRPCServer) StartJob(ctx context.Context, req *v1.StartJobRequest) (*v1.StartJobResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}

	id, err := s.registry.StartJob(req.GetExecutable(), req.GetArgs())
	if err != nil {
		return nil, jobError(err)
	}
	return &v1.StartJobResponse{JobId: id}, nil
}

func (s *GRPCServer) StopJob(ctx context.Context, req *v1.StopJobRequest) (*v1.JobStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if req.GetJobId() == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	jobStatus, err := s.registry.StopJob(req.GetJobId())
	if err != nil {
		return nil, jobError(err)
	}
	return toProtoStatus(jobStatus), nil
}

func (s *GRPCServer) GetJobStatus(ctx context.Context, req *v1.GetJobStatusRequest) (*v1.JobStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if req.GetJobId() == "" {
		return nil, status.Error(codes.InvalidArgument, "job_id is required")
	}

	jobStatus, err := s.registry.JobStatus(req.GetJobId())
	if err != nil {
		return nil, jobError(err)
	}
	return toProtoStatus(jobStatus), nil
}

func (s *GRPCServer) StreamOutput(req *v1.StreamOutputRequest, stream v1.JobService_StreamOutputServer) error {
	ctx := stream.Context()

	if req.GetJobId() == "" {
		return status.Error(codes.InvalidArgument, "job_id is required")
	}

	reader, err := s.registry.OpenStream(req.GetJobId())
	if err != nil {
		return jobError(err)
	}
	defer reader.Close()

	// Closing the reader unblocks Read when a client disconnects.
	stop := context.AfterFunc(ctx, func() { reader.Close() })
	defer stop()

	buf := make([]byte, BufferSize)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if err := stream.Send(&v1.OutputChunk{Data: bytes.Clone(buf[:n])}); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return jobError(err)
		}
	}
}

func toProtoStatus(s job.Status) *v1.JobStatus {
	result := &v1.JobStatus{
		JobId:     s.ID,
		StartedAt: timestamppb.New(s.StartedAt),
	}
	switch s.State {
	case job.StateRunning:
		result.State = v1.JobState_JOB_STATE_RUNNING
	case job.StateExited:
		result.State = v1.JobState_JOB_STATE_EXITED
	case job.StateStopped:
		result.State = v1.JobState_JOB_STATE_STOPPED
	}
	if s.FinishedAt != nil {
		result.FinishedAt = timestamppb.New(*s.FinishedAt)
	}
	if s.ExitCode != nil {
		exitCode := int32(*s.ExitCode)
		result.ExitCode = &exitCode
	}
	if s.TerminationSignal != nil {
		signal := int32(*s.TerminationSignal)
		result.TerminationSignal = &signal
	}
	return result
}
