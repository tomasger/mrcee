package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	outputPath := flag.String("o", ".local/certs/", "path to certificates")
	userName := flag.String("user", "local", "client user identity")
	role := flag.String("role", "operator", "client role: viewer or operator")
	flag.Parse()

	if err := generateCertificates(*outputPath, *userName, *role); err != nil {
		log.Fatal(err)
	}
	log.Printf("Generated CA, server, and client certificates in %s", *outputPath)
}

func generateCertificates(outDir, userName, role string) error {
	if strings.TrimSpace(userName) == "" {
		return fmt.Errorf("username cannot be empty")
	}

	if role != "viewer" && role != "operator" {
		return fmt.Errorf("role must be viewer or operator")
	}

	now := time.Now()
	ca, err := newCertificate(&x509.Certificate{
		Subject:               pkix.Name{CommonName: "mrcee development CA"},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}, nil)
	if err != nil {
		return fmt.Errorf("generate CA: %w", err)
	}

	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return fmt.Errorf("create certificate directory: %w", err)
	}
	if err := writeCertificate(outDir, "ca", ca); err != nil {
		return err
	}

	certificates := []struct {
		name     string
		template *x509.Certificate
	}{
		{
			name: "server",
			template: &x509.Certificate{
				Subject:     pkix.Name{CommonName: "mrcee server"},
				NotBefore:   now.Add(-5 * time.Minute),
				NotAfter:    now.AddDate(1, 0, 0),
				KeyUsage:    x509.KeyUsageDigitalSignature,
				ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
				DNSNames:    []string{"localhost"},
				IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
			},
		},
		{
			name: "client",
			template: &x509.Certificate{
				Subject:     pkix.Name{CommonName: userName},
				NotBefore:   now.Add(-5 * time.Minute),
				NotAfter:    now.AddDate(1, 0, 0),
				KeyUsage:    x509.KeyUsageDigitalSignature,
				ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
				URIs: []*url.URL{
					{Scheme: "urn", Opaque: "mrcee:user:" + userName},
					{Scheme: "urn", Opaque: "mrcee:role:" + role},
				},
			},
		},
	}
	for _, entry := range certificates {
		cert, err := newCertificate(entry.template, ca)
		if err != nil {
			return fmt.Errorf("generate %s certificate: %w", entry.name, err)
		}
		if err := writeCertificate(outDir, entry.name, cert); err != nil {
			return err
		}
	}
	return nil
}

type certificate struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

// newCertificate creates a self-signed certificate when parent is nil.
func newCertificate(template *x509.Certificate, parent *certificate) (*certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	if parent == nil {
		parent = &certificate{cert: template, key: key}
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, template, parent.cert, &key.PublicKey, parent.key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(certBytes)
	if err != nil {
		return nil, err
	}
	return &certificate{cert: cert, key: key}, nil
}

func writeCertificate(directory, name string, cert *certificate) error {
	pkBytes, err := x509.MarshalPKCS8PrivateKey(cert.key)
	if err != nil {
		return fmt.Errorf("encode %s private key: %w", name, err)
	}
	if err := writePEM(filepath.Join(directory, name+".key"), "PRIVATE KEY", pkBytes, 0o600); err != nil {
		return err
	}
	return writePEM(filepath.Join(directory, name+".crt"), "CERTIFICATE", cert.cert.Raw, 0o644)
}

func writePEM(path, blockType string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	// OpenFile preserves existing permissions, so also restrict keys on reruns.
	if err := file.Chmod(mode); err != nil {
		return fmt.Errorf("set permissions on %s: %w", path, err)
	}
	if err := pem.Encode(file, &pem.Block{Type: blockType, Bytes: data}); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
