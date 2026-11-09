package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const wslReleaseAPI = "https://api.github.com/repos/microsoft/WSL/releases/latest"

// Fetcher returns the body of a URL; tests replace it.
type Fetcher func(ctx context.Context, url string) ([]byte, error)

func httpFetch(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "TauShell")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 1<<20))
}

// LatestWSLVersion returns the newest stable WSL release published by Microsoft
// (GitHub's latest release excludes prereleases). It is read-only.
func LatestWSLVersion(ctx context.Context, fetch Fetcher) (string, error) {
	if fetch == nil {
		fetch = httpFetch
	}
	body, err := fetch(ctx, wslReleaseAPI)
	if err != nil {
		return "", err
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("parse WSL release: %w", err)
	}
	version := strings.TrimPrefix(strings.TrimSpace(release.Tag), "v")
	if _, ok := dottedVersion(version); !ok || release.Prerelease || release.Draft {
		return "", fmt.Errorf("unexpected WSL release %q", release.Tag)
	}
	return version, nil
}

// UpdateAvailable reports whether latest is newer than installed. Missing
// components compare as zero, so 2.6 equals 2.6.0.0. Unparseable input is
// never reported as an update.
func UpdateAvailable(installed, latest string) bool {
	have, ok1 := dottedVersion(installed)
	want, ok2 := dottedVersion(latest)
	if !ok1 || !ok2 {
		return false
	}
	for index := 0; index < len(have) || index < len(want); index++ {
		var a, b int
		if index < len(have) {
			a = have[index]
		}
		if index < len(want) {
			b = want[index]
		}
		if a != b {
			return b > a
		}
	}
	return false
}
