package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/tomasger/mrcee/internal/server"
	"github.com/tomasger/mrcee/job"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	address := flag.String("address", "127.0.0.1:10234", "ip:port to bind the server to")
	caFile := flag.String("ca", ".local/certs/ca.crt", "trusted client CA certificate or PEM bundle")
	certFile := flag.String("cert", ".local/certs/server.crt", "server TLS certificate")
	keyFile := flag.String("key", ".local/certs/server.key", "server TLS private key")
	flag.Parse()

	tlsConfig, err := loadTLSConfig(*caFile, *certFile, *keyFile)
	if err != nil {
		log.Fatalf("TLS configuration failed: %v", err)
	}

	registry := job.NewRegistry()
	srv := server.NewServer(*address, registry, grpc.Creds(credentials.NewTLS(tlsConfig)))

	// TODO: Handle termination signals and gracefully shut down the server and job registry.
	log.Printf("Starting gRPC server on %s", *address)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("gRPC server failed: %v", err)
	}
}

func loadTLSConfig(caPath, certPath, keyPath string) (*tls.Config, error) {
	if caPath == "" || certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("CA certificate, server certificate, and server key paths are required")
	}
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load server certificate and key: %w", err)
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read client CA certificates: %w", err)
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("client CA file %q contains no valid certificates", caPath)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{pair},
		ClientCAs:    clientCAs,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}, nil
}
