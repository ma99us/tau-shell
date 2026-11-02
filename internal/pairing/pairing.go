// Package pairing is the transport spike for the mobile companion: a per-pairing
// self-signed certificate that the phone pins (by SHA-256 of the certificate),
// plus a bearer token. Nothing here is wired into the app yet. The pairing
// secret (fingerprint and token) is meant to travel to the phone only in a QR
// code and must never be logged.
package pairing

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"errors"
	"math/big"
	"net"
	"net/http"
	"strings"
	"time"
)

// Credentials is one pairing: a fresh key, certificate and token.
type Credentials struct {
	Certificate tls.Certificate
	// Fingerprint is the hex SHA-256 of the DER certificate; the phone pins it.
	Fingerprint string
	// Token is the bearer secret the phone sends with each request.
	Token string
}

// New creates credentials valid for the given host names or IPs. A new pair is
// created per pairing, so no key is shared between installations and revoking a
// device means discarding its credentials.
func New(hosts []string, validFor time.Duration) (Credentials, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Credentials{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return Credentials{}, err
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "TauShell pairing"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(validFor),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return Credentials{}, err
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return Credentials{}, err
	}
	sum := sha256.Sum256(der)
	return Credentials{
		Certificate: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key},
		Fingerprint: hex.EncodeToString(sum[:]),
		Token:       hex.EncodeToString(token),
	}, nil
}

// ServerConfig is the TLS configuration for the PC side (TLS 1.3 only).
func (credentials Credentials) ServerConfig() *tls.Config {
	return &tls.Config{Certificates: []tls.Certificate{credentials.Certificate}, MinVersion: tls.VersionTLS13}
}

// RequireToken wraps a handler so only requests carrying the bearer token pass.
// The comparison is constant time.
func (credentials Credentials) RequireToken(next http.Handler) http.Handler {
	want := []byte(credentials.Token)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		got := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

// PinnedClientConfig is what the phone does: ignore the system trust store and
// accept exactly the certificate whose SHA-256 matches the pinned fingerprint.
func PinnedClientConfig(fingerprint string) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		// Chain validation is replaced by the pin below; the pin is checked on every handshake.
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return errors.New("no server certificate")
			}
			sum := sha256.Sum256(raw[0])
			if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(strings.ToLower(fingerprint))) != 1 {
				return errors.New("server certificate does not match the pinned fingerprint")
			}
			leaf, err := x509.ParseCertificate(raw[0])
			if err != nil {
				return err
			}
			if time.Now().After(leaf.NotAfter) || time.Now().Before(leaf.NotBefore) {
				return errors.New("pinned certificate is outside its validity period")
			}
			return nil
		},
	}
}
