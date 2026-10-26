package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/tomasger/mrcee/internal/testutil"
	"github.com/tomasger/mrcee/job"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// TestServerAuthenticationAndAuthorization verifies that NewServer() enforces
// authentication and authorization for all gRPC requests.
func TestServerAuthenticationAndAuthorization(t *testing.T) {
	ca := testutil.NewCertificate(t, nil, 0)
	serverPair := testutil.NewCertificate(t, &ca, x509.ExtKeyUsageServerAuth)
	roots := x509.NewCertPool()
	roots.AddCert(ca.Leaf)
	listener := bufconn.Listen(1024 * 1024)
	server := NewServer("", job.NewRegistry(), grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{serverPair},
		ClientCAs:    roots,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	})))
	go server.grpcServer.Serve(listener)
	t.Cleanup(func() {
		server.grpcServer.Stop()
		listener.Close()
	})

	tests := []struct {
		name   string
		claims []string
		read   codes.Code
		write  codes.Code
	}{
		// expected codes:
		// - unauthenticated: `codes.Unauchenticated`
		// - unauthorized: `codes.PermissionDenied`
		// - access granted: `codes.NotFound` // `codes.InvalidArgument` (as we're querying an empty registry)
		{"viewer", []string{"urn:mrcee:user:bob", "urn:mrcee:role:viewer"}, codes.NotFound, codes.PermissionDenied},
		{"operator", []string{"urn:mrcee:user:bob", "urn:mrcee:role:operator"}, codes.NotFound, codes.InvalidArgument},
		{"missing_user", []string{"urn:mrcee:role:operator"}, codes.Unauthenticated, codes.Unauthenticated},
		{"unknown_role", []string{"urn:mrcee:user:bob", "urn:mrcee:role:admin"}, codes.PermissionDenied, codes.PermissionDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pair := testutil.NewCertificate(t, &ca, x509.ExtKeyUsageClientAuth, tt.claims...)
			conn, err := grpc.NewClient("passthrough:///bufnet",
				grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
					Certificates: []tls.Certificate{pair},
					RootCAs:      roots,
					ServerName:   "localhost",
					MinVersion:   tls.VersionTLS13,
				})),
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return listener.DialContext(ctx)
				}),
			)
			require.NoError(t, err)
			defer conn.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			client := v1.NewJobServiceClient(conn)
			_, err = client.GetJobStatus(ctx, &v1.GetJobStatusRequest{JobId: "missing"})
			require.Equal(t, tt.read, status.Code(err))
			_, err = client.StartJob(ctx, &v1.StartJobRequest{})
			require.Equal(t, tt.write, status.Code(err))
			_, err = client.StopJob(ctx, &v1.StopJobRequest{})
			require.Equal(t, tt.write, status.Code(err))
			stream, err := client.StreamOutput(ctx, &v1.StreamOutputRequest{JobId: "missing"})
			require.NoError(t, err)
			_, err = stream.Recv()
			require.Equal(t, tt.read, status.Code(err))
		})
	}
}

// TestServerRejectsUnauthenticatedTransport verifies that
// interceptors reject clients if they don't have a verified certificate.
func TestServerRejectsUnauthenticatedTransport(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := NewServer("", job.NewRegistry())
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
	// expect connection to the server to be successful as we
	// do not have TLS configured.
	require.NoError(t, err)
	defer conn.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client := v1.NewJobServiceClient(conn)
	_, err = client.GetJobStatus(ctx, &v1.GetJobStatusRequest{JobId: "missing"})
	// expect `codes.Unauthenticated` because server interceptors
	// fail to locate a verified client certificate.
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	stream, err := client.StreamOutput(ctx, &v1.StreamOutputRequest{JobId: "missing"})
	require.NoError(t, err)
	_, err = stream.Recv()
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}
