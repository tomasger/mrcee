package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	v1 "github.com/tomasger/mrcee/gen/mrcee/v1"
	"github.com/urfave/cli/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// withClient supplies an authenticated client for the duration of an action.
func withClient(action func(context.Context, *cli.Command, v1.JobServiceClient) error) cli.ActionFunc {
	return func(ctx context.Context, c *cli.Command) error {
		conn, err := newClient(c)
		if err != nil {
			return err
		}
		defer conn.Close()

		return action(ctx, c, v1.NewJobServiceClient(conn))
	}
}

// newClient configures a gRPC connection
func newClient(c *cli.Command) (*grpc.ClientConn, error) {
	if c.String("server") == "" {
		return nil, fmt.Errorf("server address is required")
	}
	config, err := loadTLSConfig(c.String("ca"), c.String("cert"), c.String("key"))
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(c.String("server"), grpc.WithTransportCredentials(credentials.NewTLS(config)))
	if err != nil {
		return nil, fmt.Errorf("create gRPC client: %w", err)
	}
	return conn, nil
}

func loadTLSConfig(caPath, certPath, keyPath string) (*tls.Config, error) {
	if caPath == "" || certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("CA certificate, client certificate, and client key paths are required")
	}
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load client certificate and key: %w", err)
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read server CA certificates: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("server CA file %q contains no valid certificates", caPath)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{pair},
		RootCAs:      roots,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
