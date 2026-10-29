package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/tomasger/mrcee/internal/testutil"
	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	grpcstatus "google.golang.org/grpc/status"
)

func writeTestCertificate(t *testing.T, directory, name string, cert tls.Certificate) {
	t.Helper()
	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(directory, name+".crt"),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(directory, name+".key"),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600))
}

func TestWithClientMutualTLS(t *testing.T) {
	directory := t.TempDir()
	ca := testutil.NewCertificate(t, nil, 0)
	writeTestCertificate(t, directory, "ca", ca)
	writeTestCertificate(t, directory, "client", testutil.NewCertificate(t, &ca, x509.ExtKeyUsageClientAuth))
	roots := x509.NewCertPool()
	roots.AddCert(ca.Leaf)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{testutil.NewCertificate(t, &ca, x509.ExtKeyUsageServerAuth)},
		ClientCAs:    roots,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	})))
	// Use Unimplemented - we're only testing RPC transport via TLS.
	v1.RegisterJobServiceServer(grpcServer, &v1.UnimplementedJobServiceServer{})
	go grpcServer.Serve(listener)
	t.Cleanup(grpcServer.Stop)
	_, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	otherCA := testutil.NewCertificate(t, nil, 0)
	writeTestCertificate(t, directory, "other-ca", otherCA)
	writeTestCertificate(t, directory, "untrusted-client", testutil.NewCertificate(t, &otherCA, x509.ExtKeyUsageClientAuth))
	actionErr := errors.New("action failed")
	type testCase struct {
		name      string
		host      string
		ca        string
		client    string
		wantCode  codes.Code
		actionErr error
	}
	testCases := []testCase{
		{
			name:     "trusted",
			host:     "localhost",
			ca:       "ca",
			client:   "client",
			wantCode: codes.Unimplemented,
		},
		{
			name:      "action_error",
			host:      "localhost",
			ca:        "ca",
			client:    "client",
			wantCode:  codes.Unimplemented,
			actionErr: actionErr,
		},
		{
			name:     "wrong_hostname",
			host:     "127.0.0.1",
			ca:       "ca",
			client:   "client",
			wantCode: codes.Unavailable,
		},
		{
			name:     "untrusted_server",
			host:     "localhost",
			ca:       "other-ca",
			client:   "client",
			wantCode: codes.Unavailable,
		},
		{
			name:     "untrusted_client",
			host:     "localhost",
			ca:       "ca",
			client:   "untrusted-client",
			wantCode: codes.Unavailable,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var client v1.JobServiceClient
			cmd := &cli.Command{
				Name: "test",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "server", Value: net.JoinHostPort(tc.host, port)},
					&cli.StringFlag{Name: "ca", Value: filepath.Join(directory, tc.ca+".crt")},
					&cli.StringFlag{Name: "cert", Value: filepath.Join(directory, tc.client+".crt")},
					&cli.StringFlag{Name: "key", Value: filepath.Join(directory, tc.client+".key")},
				},
				Action: withClient(func(ctx context.Context, _ *cli.Command, connected v1.JobServiceClient) error {
					client = connected
					_, err := client.GetJobStatus(ctx, &v1.GetJobStatusRequest{JobId: "missing"})
					require.Equal(t, tc.wantCode, grpcstatus.Code(err))
					return tc.actionErr
				}),
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			err := cmd.Run(ctx, []string{"test"})
			require.ErrorIs(t, err, tc.actionErr)
			require.NotNil(t, client)
			// Closing the connection must also happen when an action returns an error.
			_, err = client.GetJobStatus(ctx, &v1.GetJobStatusRequest{JobId: "missing"})
			require.Equal(t, codes.Canceled, grpcstatus.Code(err))
		})
	}
}

func TestLoadTLSConfigRejectsInvalidFiles(t *testing.T) {
	directory := t.TempDir()
	ca := testutil.NewCertificate(t, nil, 0)
	writeTestCertificate(t, directory, "ca", ca)
	writeTestCertificate(t, directory, "client", testutil.NewCertificate(t, &ca, x509.ExtKeyUsageClientAuth))
	caPath := filepath.Join(directory, "ca.crt")
	certPath := filepath.Join(directory, "client.crt")
	keyPath := filepath.Join(directory, "client.key")
	invalid := filepath.Join(directory, "invalid.file")
	require.NoError(t, os.WriteFile(invalid, []byte("invalid PEM"), 0o600))

	type testCase struct {
		name string
		ca   string
		cert string
		key  string
	}
	testCases := []testCase{
		{
			name: "empty_ca",
			ca:   "",
			cert: certPath,
			key:  keyPath,
		},
		{
			name: "empty_cert",
			ca:   caPath,
			cert: "",
			key:  keyPath,
		},
		{
			name: "empty_key",
			ca:   caPath,
			cert: certPath,
			key:  "",
		},
		{
			name: "invalid_ca",
			ca:   invalid,
			cert: certPath,
			key:  keyPath,
		},
		{
			name: "invalid_cert",
			ca:   caPath,
			cert: invalid,
			key:  keyPath,
		},
		{
			name: "invalid_key",
			ca:   caPath,
			cert: certPath,
			key:  invalid,
		},
		{
			name: "mismatched_key",
			ca:   caPath,
			cert: certPath,
			key:  filepath.Join(directory, "ca.key"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config, err := loadTLSConfig(tc.ca, tc.cert, tc.key)
			require.Error(t, err)
			require.Nil(t, config)
		})
	}
}
