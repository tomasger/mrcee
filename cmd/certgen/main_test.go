package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func loadCertFromFile(t *testing.T, directory, name string) tls.Certificate {
	t.Helper()
	certPath := filepath.Join(directory, name+".crt")
	keyPath := filepath.Join(directory, name+".key")
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err)
	require.NotNil(t, pair.Leaf)
	return pair
}

// TestGenerateCertificatesVerification checks that generated certificates use
// ECDSA P-256 keys, have the expected key usages and SANs, and verify against
// the generated CA. It also checks private key file permissions.
func TestGenerateCertificatesVerification(t *testing.T) {
	testCases := []struct {
		name string
		user string
		role string
	}{
		{
			name: "operator",
			user: "alice",
			role: "operator",
		},
		{
			name: "viewer",
			user: "bob",
			role: "viewer",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "certs")
			require.NoError(t, generateCertificates(directory, tc.user, tc.role))

			loadCertificate := func(name string) tls.Certificate {
				t.Helper()
				pair := loadCertFromFile(t, directory, name)
				keyPath := filepath.Join(directory, name+".key")
				require.IsType(t, &ecdsa.PrivateKey{}, pair.PrivateKey)
				require.Equal(t, elliptic.P256(), pair.PrivateKey.(*ecdsa.PrivateKey).Curve)
				keyInfo, err := os.Stat(keyPath)
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0o600), keyInfo.Mode().Perm())
				return pair
			}

			ca := loadCertificate("ca")
			serverPair := loadCertificate("server")
			clientPair := loadCertificate("client")
			require.True(t, ca.Leaf.IsCA)
			require.NoError(t, ca.Leaf.CheckSignatureFrom(ca.Leaf))
			roots := x509.NewCertPool()
			roots.AddCert(ca.Leaf)

			serverNames := []string{"localhost", "127.0.0.1", "::1"}
			for _, name := range serverNames {
				_, err := serverPair.Leaf.Verify(x509.VerifyOptions{
					Roots:     roots,
					DNSName:   name,
					KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
				})
				require.NoError(t, err)
			}
			_, err := clientPair.Leaf.Verify(x509.VerifyOptions{
				Roots:     roots,
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			})
			require.NoError(t, err)
			_, err = serverPair.Leaf.Verify(x509.VerifyOptions{
				Roots:     roots,
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			})
			require.Error(t, err, "server certificate must not authenticate as a client")
			_, err = clientPair.Leaf.Verify(x509.VerifyOptions{
				Roots:     roots,
				KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			})
			require.Error(t, err, "client certificate must not authenticate as a server")
			require.Len(t, clientPair.Leaf.URIs, 2)
			require.Equal(t, "urn:mrcee:user:"+tc.user, clientPair.Leaf.URIs[0].String())
			require.Equal(t, "urn:mrcee:role:"+tc.role, clientPair.Leaf.URIs[1].String())
		})
	}
}

// TestGenerateCertificatesTLS checks that generated certificates support
// mTLS 1.3 authentication and data exchange.
func TestGenerateCertificatesTLS(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "certs")
	require.NoError(t, generateCertificates(directory, "alice", "operator"))
	ca := loadCertFromFile(t, directory, "ca")
	serverPair := loadCertFromFile(t, directory, "server")
	clientPair := loadCertFromFile(t, directory, "client")
	roots := x509.NewCertPool()
	roots.AddCert(ca.Leaf)

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()
	deadline := time.Now().Add(5 * time.Second)
	require.NoError(t, serverConn.SetDeadline(deadline))
	require.NoError(t, clientConn.SetDeadline(deadline))
	server := tls.Server(serverConn, &tls.Config{
		Certificates:           []tls.Certificate{serverPair},
		ClientCAs:              roots,
		ClientAuth:             tls.RequireAndVerifyClientCert,
		MinVersion:             tls.VersionTLS13,
		MaxVersion:             tls.VersionTLS13,
		SessionTicketsDisabled: true,
	})
	client := tls.Client(clientConn, &tls.Config{
		Certificates: []tls.Certificate{clientPair},
		RootCAs:      roots,
		ServerName:   "localhost",
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
	})
	payload := []byte("hello over mTLS")

	// setup server to echo back same message it receives
	serverDone := make(chan error, 1)
	go func() {
		data := make([]byte, len(payload))
		if _, err := io.ReadFull(server, data); err != nil {
			serverDone <- err
			return
		}
		_, err := server.Write(data)
		serverDone <- err
	}()

	// Performing read/write ensures TLS handshake occurs.
	n, err := client.Write(payload)
	require.NoError(t, err)
	require.Equal(t, len(payload), n)
	response := make([]byte, len(payload))
	_, err = io.ReadFull(client, response)
	require.NoError(t, err)
	require.Equal(t, payload, response)
	require.NoError(t, <-serverDone)
	require.Equal(t, uint16(tls.VersionTLS13), server.ConnectionState().Version)
	require.NotEmpty(t, server.ConnectionState().VerifiedChains)
	require.NotEmpty(t, client.ConnectionState().VerifiedChains)
}

func TestGenerateCertificatesRejectsInvalidIdentity(t *testing.T) {
	testCases := []struct {
		name string
		user string
		role string
	}{
		{
			name: "empty_user",
			user: "",
			role: "operator",
		},
		{
			name: "empty_role",
			user: "bob",
			role: "",
		},
		{
			name: "unknown_role",
			user: "alice",
			role: "admin",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, generateCertificates(t.TempDir(), tc.user, tc.role))
		})
	}
}

func TestGenerateCertificatesRestrictsExistingKeyPermissions(t *testing.T) {
	directory := t.TempDir()
	keyPath := filepath.Join(directory, "client.key")
	require.NoError(t, os.WriteFile(keyPath, nil, 0o644))
	require.NoError(t, os.Chmod(keyPath, 0o644))
	require.NoError(t, generateCertificates(directory, "alice", "operator"))
	info, err := os.Stat(keyPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
