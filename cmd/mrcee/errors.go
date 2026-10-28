package main

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// rpcError translates RPC failures into messages for CLI users.
func rpcError(err error) error {
	if err == nil {
		return nil
	}
	s, ok := grpcstatus.FromError(err)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		s = grpcstatus.FromContextError(err)
	} else if !ok {
		return err
	}

	var message string
	switch s.Code() {
	case codes.InvalidArgument:
		message = "invalid request: " + s.Message()
	case codes.NotFound:
		message = "job not found"
	case codes.FailedPrecondition:
		message = s.Message()
	case codes.Canceled:
		message = "request canceled"
	case codes.DeadlineExceeded:
		message = "request timed out"
	case codes.Internal:
		message = "the server could not complete the request; contact server administrator"
	case codes.Unauthenticated:
		message = "authentication failed; ensure your client certificate is correct"
	case codes.PermissionDenied:
		message = "permission denied; check the role in your client certificate"
	case codes.Unavailable:
		message = "server unavailable; check your connection and value of --server"
	case codes.Unimplemented:
		message = "the server does not support this operation; check client and server versions"
	default:
		message = fmt.Sprintf("request failed (%s): %s", s.Code(), s.Message())
	}
	return errors.New(message)
}
