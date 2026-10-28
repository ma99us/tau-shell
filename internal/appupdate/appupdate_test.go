package appupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		current, tag string
		want         bool
	}{
		{"1.0.0", "v1.0.1", true},
		{"1.0.0", "v1.0.0", false},
		{"1.2.0", "v1.10.0", true},
		{"2.0.0", "v1.9.9", false},
		{"1.0.0-rc1", "v1.0.0", false},
		{"dev", "v9.9.9", false},
		{"1.0.0", "nightly", false},
	}
	for _, c := range cases {
		if got := Newer(c.current, c.tag); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.current, c.tag, got, c.want)
		}
	}
}

func TestLatestPicksHighestNonDraftWithTrustedURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"v1.2.0","html_url":"` + ReleasesPrefix + `/tag/v1.2.0","draft":true},
			{"tag_name":"v1.1.0","html_url":"` + ReleasesPrefix + `/tag/v1.1.0"},
			{"tag_name":"v1.10.0","html_url":"https://evil.example/x"},
			{"tag_name":"v1.0.0","html_url":"` + ReleasesPrefix + `/tag/v1.0.0"}]`))
	}))
	defer server.Close()
	release, ok, err := Latest(context.Background(), server.Client(), server.URL)
	if err != nil || !ok {
		t.Fatalf("Latest() = %v, %v, %v", release, ok, err)
	}
	if release.Tag != "v1.1.0" {
		t.Fatalf("tag = %q, want v1.1.0", release.Tag)
	}
}

func TestLatestReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer server.Close()
	if _, _, err := Latest(context.Background(), server.Client(), server.URL); err == nil {
		t.Fatal("expected an error for HTTP 403")
	}
}
