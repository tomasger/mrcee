package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/url"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func certificateContext(t *testing.T, claims ...string) context.Context {
	t.Helper()
	cert := &x509.Certificate{}
	for _, claim := range claims {
		uri, err := url.Parse(claim)
		require.NoError(t, err)
		cert.URIs = append(cert.URIs, uri)
	}
	return peer.NewContext(t.Context(), &peer.Peer{AuthInfo: credentials.TLSInfo{
		State: tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{cert}}},
	}})
}

type interceptorTestStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s interceptorTestStream) Context() context.Context { return s.ctx }

func TestAuthenticationInterceptors(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		code codes.Code
	}{
		{"valid_identity", certificateContext(t, "urn:mrcee:user:bob"), codes.OK},
		{"numeric_username", certificateContext(t, "urn:mrcee:user:123"), codes.OK},
		{"unrelated_uri", certificateContext(t, "urn:other:user:alice", "urn:mrcee:user:bob"), codes.OK},
		{"no_identity", t.Context(), codes.Unauthenticated},
		{"no_auth_info", peer.NewContext(t.Context(), &peer.Peer{}), codes.Unauthenticated},
		{"unverified_certificate", peer.NewContext(t.Context(), &peer.Peer{AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{{}}},
		}}), codes.Unauthenticated},
		{"empty_chain", peer.NewContext(t.Context(), &peer.Peer{AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{}}},
		}}), codes.Unauthenticated},
		{"missing_identity", certificateContext(t, "urn:mrcee:role:operator"), codes.Unauthenticated},
		{"bare_user_prefix", certificateContext(t, "urn:mrcee:user"), codes.Unauthenticated},
		{"missing_separator", certificateContext(t, "urn:mrcee:userbob"), codes.Unauthenticated},
		{"missing_username", certificateContext(t, "urn:mrcee:user:"), codes.Unauthenticated},
		{"whitespace_username", certificateContext(t, "urn:mrcee:user:   "), codes.Unauthenticated},
		{"wrong_prefix", certificateContext(t, "urn:other:user:bob"), codes.Unauthenticated},
		{"multiple_users", certificateContext(t, "urn:mrcee:user:bob", "urn:mrcee:user:alice"), codes.Unauthenticated},
		{"valid_then_empty_user", certificateContext(t, "urn:mrcee:user:bob", "urn:mrcee:user:"), codes.Unauthenticated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			_, err := UnaryAuthenticationInterceptor(tt.ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
				called = true
				return nil, nil
			})

			require.Equal(t, tt.code, status.Code(err))
			require.Equal(t, tt.code == codes.OK, called)

			called = false
			stream := interceptorTestStream{ctx: tt.ctx}
			err = StreamAuthenticationInterceptor(nil, stream, &grpc.StreamServerInfo{}, func(srv any, got grpc.ServerStream) error {
				called = true
				require.Equal(t, stream, got)
				return nil
			})
			require.Equal(t, tt.code, status.Code(err))
			require.Equal(t, tt.code == codes.OK, called)
		})
	}
}

func TestAuthorizationInterceptors(t *testing.T) {
	readMethods := []string{
		v1.JobService_GetJobStatus_FullMethodName,
		v1.JobService_StreamOutput_FullMethodName,
	}
	writeMethods := []string{
		v1.JobService_StartJob_FullMethodName,
		v1.JobService_StopJob_FullMethodName,
	}
	unknownMethods := []string{
		"/mrcee.v1.JobService/Unknown",
	}
	roles := []struct {
		name         string
		claims       []string
		readAllowed  bool
		writeAllowed bool
	}{
		{"viewer", []string{"urn:mrcee:role:viewer"}, true, false},
		{"operator", []string{"urn:mrcee:role:operator"}, true, true},
		{"missing_role", nil, false, false},
		{"empty_role", []string{"urn:mrcee:role:"}, false, false},
		{"whitespace_role", []string{"urn:mrcee:role:   "}, false, false},
		{"unknown_role", []string{"urn:mrcee:role:admin"}, false, false},
		{"role_suffix", []string{"urn:mrcee:role:operator-extra"}, false, false},
		{"multiple_roles", []string{"urn:mrcee:role:viewer", "urn:mrcee:role:operator"}, false, false},
		{"duplicate_role", []string{"urn:mrcee:role:operator", "urn:mrcee:role:operator"}, false, false},
		{"valid_then_empty_role", []string{"urn:mrcee:role:operator", "urn:mrcee:role:"}, false, false},
		{"empty_then_valid_role", []string{"urn:mrcee:role:", "urn:mrcee:role:operator"}, false, false},
		{"unknown_then_operator", []string{"urn:mrcee:role:admin", "urn:mrcee:role:operator"}, false, false},
		{"operator_then_unknown", []string{"urn:mrcee:role:operator", "urn:mrcee:role:admin"}, false, false},
	}

	for _, role := range roles {
		groups := []struct {
			name    string
			methods []string
			allowed bool
		}{
			{"read", readMethods, role.readAllowed},
			{"write", writeMethods, role.writeAllowed},
			{"unknown", unknownMethods, false},
		}
		for _, group := range groups {
			for _, method := range group.methods {
				t.Run(role.name+"/"+group.name+"/"+path.Base(method), func(t *testing.T) {
					ctx := certificateContext(t, append([]string{"urn:mrcee:user:bob"}, role.claims...)...)
					want := codes.PermissionDenied
					if group.allowed {
						want = codes.OK
					}

					t.Run("unary", func(t *testing.T) {
						called := false
						_, err := UnaryAuthorizationInterceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(context.Context, any) (any, error) {
							called = true
							return nil, nil
						})
						require.Equal(t, want, status.Code(err))
						require.Equal(t, group.allowed, called)
					})

					t.Run("stream", func(t *testing.T) {
						called := false
						err := StreamAuthorizationInterceptor(nil, interceptorTestStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: method}, func(any, grpc.ServerStream) error {
							called = true
							return nil
						})
						require.Equal(t, want, status.Code(err))
						require.Equal(t, group.allowed, called)
					})
				})
			}
		}
	}
}
