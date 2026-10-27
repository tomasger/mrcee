package server

import (
	"errors"
	"io"
	"os"
	"os/exec"

	"github.com/tomasger/mrcee/job"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// jobError translates known errors from the job library
// to corresponding gRPC error codes.
// TODO: introduce error propagation inside gRPC messages for
// more detailed error overview.
func jobError(err error) error {
	switch {
	case errors.Is(err, job.ErrEmptyCommand):
		return status.Error(codes.InvalidArgument, "executable is required")
	case errors.Is(err, exec.ErrNotFound), errors.Is(err, os.ErrNotExist):
		return status.Error(codes.InvalidArgument, "executable not found")
	case errors.Is(err, job.ErrJobNotFound):
		return status.Error(codes.NotFound, "job not found")
	case errors.Is(err, job.ErrJobAlreadyStopped):
		return status.Error(codes.FailedPrecondition, "job has already finished")
	case errors.Is(err, io.ErrClosedPipe):
		return status.Error(codes.Canceled, "output stream canceled")
	default:
		return status.Error(codes.Internal, "internal server error")
	}
}
