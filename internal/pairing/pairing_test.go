package pairing

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func serve(t *testing.T, credentials Credentials) string {
	t.Helper()
	server := httptest.NewUnstartedServer(credentials.RequireToken(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "ok")
	})))
	server.TLS = credentials.ServerConfig()
	server.StartTLS()
	t.Cleanup(server.Close)
	return server.URL
}

func get(url string, config *tls.Config, token string) (int, error) {
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: config}, Timeout: 5 * time.Second}
	request, _ := http.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}

func TestPinnedClientReachesServerWithToken(t *testing.T) {
	credentials, err := New([]string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	url := serve(t, credentials)
	if status, err := get(url, PinnedClientConfig(credentials.Fingerprint), credentials.Token); err != nil || status != 200 {
		t.Fatalf("pinned client with token: %d %v", status, err)
	}
	if status, err := get(url, PinnedClientConfig(credentials.Fingerprint), "wrong"); err != nil || status != 401 {
		t.Fatalf("wrong token must be rejected: %d %v", status, err)
	}
	if status, _ := get(url, PinnedClientConfig(credentials.Fingerprint), ""); status != 401 {
		t.Fatalf("missing token must be rejected: %d", status)
	}
}

func TestDifferentPairingOrSystemTrustCannotConnect(t *testing.T) {
	first, _ := New([]string{"127.0.0.1"}, time.Hour)
	other, _ := New([]string{"127.0.0.1"}, time.Hour)
	if first.Fingerprint == other.Fingerprint || first.Token == other.Token {
		t.Fatal("every pairing must get its own key material")
	}
	url := serve(t, first)
	// An impostor presenting another pairing's certificate is refused by the pin.
	if _, err := get(url, PinnedClientConfig(other.Fingerprint), first.Token); err == nil {
		t.Fatal("a client pinned to a different certificate must fail the handshake")
	}
	// A client without the pin falls back to the system trust store and rejects a self-signed cert.
	if _, err := get(url, &tls.Config{MinVersion: tls.VersionTLS13}, first.Token); err == nil {
		t.Fatal("an unpinned client must not trust the certificate")
	}
}

func TestExpiredCertificateIsRefusedAndTLS12IsNot_Offered(t *testing.T) {
	expired, err := New([]string{"127.0.0.1"}, -time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	url := serve(t, expired)
	if _, err := get(url, PinnedClientConfig(expired.Fingerprint), expired.Token); err == nil {
		t.Fatal("an expired pinned certificate must be refused")
	}
	live, _ := New([]string{"127.0.0.1"}, time.Hour)
	address := serve(t, live)[len("https://"):]
	connection, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", address, &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12})
	if err == nil {
		connection.Close()
		t.Fatal("TLS 1.2 must not be accepted")
	}
}
