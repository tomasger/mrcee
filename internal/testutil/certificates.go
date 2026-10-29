// Package testutil provides fixtures shared by tests in multiple packages.
package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// NewCertificate creates a signed test certificate with the given usage and URI
// claims. If parent==nil, it creates a self-signed CA instead. Server certificates are
// valid for localhost.
func NewCertificate(t *testing.T, parent *tls.Certificate, usage x509.ExtKeyUsage, claims ...string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: serial,
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	for _, claim := range claims {
		uri, err := url.Parse(claim)
		require.NoError(t, err)
		template.URIs = append(template.URIs, uri)
	}
	issuer := template
	var issuerKey any = key
	if parent == nil {
		template.IsCA = true
		template.BasicConstraintsValid = true
		template.KeyUsage |= x509.KeyUsageCertSign
	} else {
		issuer, issuerKey = parent.Leaf, parent.PrivateKey
		template.ExtKeyUsage = []x509.ExtKeyUsage{usage}
		if usage == x509.ExtKeyUsageServerAuth {
			template.DNSNames = []string{"localhost"}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, issuer, &key.PublicKey, issuerKey)
	require.NoError(t, err)
	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}
