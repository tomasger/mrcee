package server

import (
	"context"
	"crypto/x509"
	"slices"
	"strings"

	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	sanUsernamePrefix = "urn:mrcee:user:"
	sanRolePrefix     = "urn:mrcee:role:"
)

// roleMethods map roles to the methods they can execute.
func roleMethods() map[string][]string {
	return map[string][]string{
		"viewer": {
			v1.JobService_GetJobStatus_FullMethodName,
			v1.JobService_StreamOutput_FullMethodName,
		},
		"operator": {
			v1.JobService_StartJob_FullMethodName,
			v1.JobService_StopJob_FullMethodName,
			v1.JobService_GetJobStatus_FullMethodName,
			v1.JobService_StreamOutput_FullMethodName,
		},
	}
}

// UnaryAuthenticationInterceptor requires a verified client certificate with a
// single, non-empty urn:mrcee:user:<username> URI SAN.
func UnaryAuthenticationInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if err := authenticate(ctx); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

// StreamAuthenticationInterceptor authenticates streaming RPCs.
func StreamAuthenticationInterceptor(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := authenticate(stream.Context()); err != nil {
		return err
	}
	return handler(srv, stream)
}

// UnaryAuthorizationInterceptor applies the viewer/operator permissions to unary
// RPCs. Authentication must run first.
func UnaryAuthorizationInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if err := authorize(ctx, info.FullMethod); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

// StreamAuthorizationInterceptor applies role permissions to streaming RPCs.
// Authentication must run first.
func StreamAuthorizationInterceptor(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := authorize(stream.Context(), info.FullMethod); err != nil {
		return err
	}
	return handler(srv, stream)
}

func authenticate(ctx context.Context) error {
	cert, err := verifiedClientCertificate(ctx)
	if err != nil {
		return err
	}
	if _, ok := certificateClaim(cert, sanUsernamePrefix); !ok {
		return status.Error(codes.Unauthenticated, "client certificate must contain one non-empty user identity")
	}
	return nil
}

// authorize extracts role claim from the certificate
// and checks whether access to a resource will be granted.
// `viewer` role can only access GetJobStatus and StreamOutput.
// `operator` role can also access StartJob and StopJob.
// If any new method is added it will be denied by default until
// role access to it is configured here.
func authorize(ctx context.Context, method string) error {
	cert, err := verifiedClientCertificate(ctx)
	if err != nil {
		return err
	}
	role, ok := certificateClaim(cert, sanRolePrefix)
	allowedMethods, isValidRole := roleMethods()[role]
	if !ok || !isValidRole {
		return status.Error(codes.PermissionDenied, "client certificate must contain exactly one supported role")
	}
	if !slices.Contains(allowedMethods, method) {
		return status.Error(codes.PermissionDenied, "role does not permit this method")
	}
	return nil
}

// verifiedClientCertificate returns the client's leaf certificate from the TLS
// verified chains attached to the gRPC peer.
func verifiedClientCertificate(ctx context.Context) (*x509.Certificate, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "verified client certificate is required")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.VerifiedChains) == 0 || len(tlsInfo.State.VerifiedChains[0]) == 0 || tlsInfo.State.VerifiedChains[0][0] == nil {
		return nil, status.Error(codes.Unauthenticated, "verified client certificate is required")
	}
	return tlsInfo.State.VerifiedChains[0][0], nil
}

// certificateClaim requires exactly one non-empty claim with the given prefix.
// Repeated claims are rejected even when their values match.
func certificateClaim(cert *x509.Certificate, prefix string) (string, bool) {
	var value string
	found := false
	for _, uri := range cert.URIs {
		if uri == nil {
			continue
		}
		claim, ok := strings.CutPrefix(uri.String(), prefix)
		if !ok {
			continue
		}
		if found || strings.TrimSpace(claim) == "" {
			return "", false
		}
		value, found = claim, true
	}
	return value, found
}
