package server

import (
	"fmt"
	"net"

	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/tomasger/mrcee/job"
	"google.golang.org/grpc"
)

// Server hosts the gRPC job service.
type Server struct {
	address    string
	grpcServer *grpc.Server
}

// NewServer installs authentication and role authorization for every RPC.
// Callers must supply transport credentials that verify client certificates.
func NewServer(address string, registry *job.Registry, opts ...grpc.ServerOption) *Server {
	opts = append([]grpc.ServerOption{
		grpc.ChainUnaryInterceptor(UnaryAuthenticationInterceptor, UnaryAuthorizationInterceptor),
		grpc.ChainStreamInterceptor(StreamAuthenticationInterceptor, StreamAuthorizationInterceptor),
	}, opts...)
	grpcServer := grpc.NewServer(opts...)
	v1.RegisterJobServiceServer(grpcServer, NewGRPCServer(registry))
	return &Server{address: address, grpcServer: grpcServer}
}

// ListenAndServe binds the TCP address and blocks while serving gRPC requests.
func (s *Server) ListenAndServe() error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.address, err)
	}
	defer listener.Close()

	return s.grpcServer.Serve(listener)
}
