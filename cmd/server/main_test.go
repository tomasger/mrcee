package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/tomasger/mrcee/internal/server"
	"github.com/tomasger/mrcee/internal/testutil"
	"github.com/tomasger/mrcee/job"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func writeTestCertificate(t *testing.T, directory, name string, cert tls.Certificate) {
	t.Helper()
	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	require.NoError(t, os.WriteFile(filepath.Join(directory, name+".crt"), certPEM, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(directory, name+".key"), keyPEM, 0o600))
}

func TestLoadTLSConfigMutualTLS(t *testing.T) {
	directory := t.TempDir()
	ca := testutil.NewCertificate(t, nil, 0)
	writeTestCertificate(t, directory, "ca", ca)
	writeTestCertificate(t, directory, "server", testutil.NewCertificate(t, &ca, x509.ExtKeyUsageServerAuth))
	clientPair := testutil.NewCertificate(t, &ca, x509.ExtKeyUsageClientAuth)
	untrustedCA := testutil.NewCertificate(t, nil, 0)
	untrustedClient := testutil.NewCertificate(t, &untrustedCA, x509.ExtKeyUsageClientAuth)
	serverTLS, err := loadTLSConfig(
		filepath.Join(directory, "ca.crt"),
		filepath.Join(directory, "server.crt"),
		filepath.Join(directory, "server.key"),
	)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(ca.Leaf)

	listener := bufconn.Listen(1024 * 1024)
	// Only exercise the loaded TLS configuration here; application permissions
	// are covered by internal/server/server_test.go.
	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
	v1.RegisterJobServiceServer(grpcServer, server.NewGRPCServer(job.NewRegistry()))
	go grpcServer.Serve(listener)
	t.Cleanup(func() {
		grpcServer.Stop()
		listener.Close()
	})

	testCases := []struct {
		name        string
		certificate *tls.Certificate
		version     uint16
		wantCode    codes.Code
	}{
		{
			name:        "trusted_client",
			certificate: &clientPair,
			version:     tls.VersionTLS13,
			// NotFound means TLS succeeded and the request reached the job service.
			wantCode: codes.NotFound,
		},
		{
			name:     "missing_client_certificate",
			version:  tls.VersionTLS13,
			wantCode: codes.Unavailable,
		},
		{
			name:        "untrusted_client",
			certificate: &untrustedClient,
			version:     tls.VersionTLS13,
			wantCode:    codes.Unavailable,
		},
		{
			name:        "tls_1_2",
			certificate: &clientPair,
			version:     tls.VersionTLS12,
			wantCode:    codes.Unavailable,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := grpc.NewClient("passthrough:///bufnet",
				grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
					RootCAs:    roots,
					ServerName: "localhost",
					MinVersion: tc.version,
					MaxVersion: tc.version,
					// Present untrusted certificates too, so the server must reject them.
					GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
						if tc.certificate == nil {
							return &tls.Certificate{}, nil
						}
						return tc.certificate, nil
					},
				})),
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return listener.DialContext(ctx)
				}),
			)
			require.NoError(t, err)
			defer conn.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			_, err = v1.NewJobServiceClient(conn).GetJobStatus(ctx, &v1.GetJobStatusRequest{JobId: "missing"})
			require.Equal(t, tc.wantCode, status.Code(err))
		})
	}
}

func TestLoadTLSConfigRejectsInvalidFiles(t *testing.T) {
	directory := t.TempDir()
	ca := testutil.NewCertificate(t, nil, 0)
	writeTestCertificate(t, directory, "ca", ca)
	writeTestCertificate(t, directory, "server", testutil.NewCertificate(t, &ca, x509.ExtKeyUsageServerAuth))
	caPath := filepath.Join(directory, "ca.crt")
	certPath := filepath.Join(directory, "server.crt")
	keyPath := filepath.Join(directory, "server.key")
	missingPath := filepath.Join(directory, "missing")
	invalidPath := filepath.Join(directory, "invalid.pem")
	require.NoError(t, os.WriteFile(invalidPath, []byte("invalid PEM"), 0o600))

	testCases := []struct {
		name     string
		caPath   string
		certPath string
		keyPath  string
	}{
		{
			name:     "empty_ca_path",
			caPath:   "",
			certPath: certPath,
			keyPath:  keyPath,
		},
		{
			name:     "empty_certificate_path",
			caPath:   caPath,
			certPath: "",
			keyPath:  keyPath,
		},
		{
			name:     "empty_key_path",
			caPath:   caPath,
			certPath: certPath,
			keyPath:  "",
		},
		{
			name:     "missing_ca",
			caPath:   missingPath,
			certPath: certPath,
			keyPath:  keyPath,
		},
		{
			name:     "missing_certificate",
			caPath:   caPath,
			certPath: missingPath,
			keyPath:  keyPath,
		},
		{
			name:     "missing_key",
			caPath:   caPath,
			certPath: certPath,
			keyPath:  missingPath,
		},
		{
			name:     "invalid_ca",
			caPath:   invalidPath,
			certPath: certPath,
			keyPath:  keyPath,
		},
		{
			name:     "invalid_certificate",
			caPath:   caPath,
			certPath: invalidPath,
			keyPath:  keyPath,
		},
		{
			name:     "invalid_key",
			caPath:   caPath,
			certPath: certPath,
			keyPath:  invalidPath,
		},
		{
			name:     "mismatched_key",
			caPath:   caPath,
			certPath: certPath,
			keyPath:  filepath.Join(directory, "ca.key"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config, err := loadTLSConfig(tc.caPath, tc.certPath, tc.keyPath)
			require.Error(t, err)
			require.Nil(t, config)
		})
	}
}
