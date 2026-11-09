package runtime

import (
	"context"
	"errors"
	"testing"
)

func TestUpdateAvailable(t *testing.T) {
	cases := []struct {
		installed, latest string
		want              bool
	}{
		{"2.6.2.0", "2.6.3", true},
		{"2.6.3.0", "2.6.3", false},
		{"2.6.3", "2.6.3.0", false},
		{"2.7.0.0", "2.6.9", false},
		{"2.6.2.0", "2.10.0", true},
		{"", "2.6.3", false},
		{"2.6.2.0", "next", false},
	}
	for _, test := range cases {
		if got := UpdateAvailable(test.installed, test.latest); got != test.want {
			t.Errorf("UpdateAvailable(%q, %q) = %v", test.installed, test.latest, got)
		}
	}
}

func TestLatestWSLVersion(t *testing.T) {
	serve := func(body string, err error) Fetcher {
		return func(context.Context, string) ([]byte, error) { return []byte(body), err }
	}
	if got, err := LatestWSLVersion(context.Background(), serve(`{"tag_name":"v2.6.3","prerelease":false}`, nil)); err != nil || got != "2.6.3" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, body := range []string{`{"tag_name":"2.7.0","prerelease":true}`, `{"tag_name":"nightly"}`, `not json`} {
		if _, err := LatestWSLVersion(context.Background(), serve(body, nil)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	if _, err := LatestWSLVersion(context.Background(), serve("", errors.New("offline"))); err == nil {
		t.Error("fetch error swallowed")
	}
}
