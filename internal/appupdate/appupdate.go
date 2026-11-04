// Package appupdate checks GitHub Releases for a newer TauShell build and can
// download, verify and install it (see install.go). Nothing runs without the
// caller asking for it.
package appupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// ReleasesAPI lists the project's releases, newest first.
	ReleasesAPI = "https://api.github.com/repos/ma99us/tau-shell/releases?per_page=20"
	// ReleasesPrefix is the only URL prefix a reported release page may have.
	ReleasesPrefix = "https://github.com/ma99us/tau-shell/releases"
	// DownloadPrefix is the only URL prefix release assets may be fetched from.
	DownloadPrefix = ReleasesPrefix + "/download/"
	// SumsName is the release asset listing the SHA-256 of the ZIP.
	SumsName = "SHA256SUMS.txt"

	requestTimeout = 10 * time.Second
	maxBody        = 1 << 20
)

// Release is the newest published release.
type Release struct {
	Tag     string `json:"tag"`
	URL     string `json:"url"`
	Notes   string `json:"notes"`
	ZipURL  string `json:"-"`
	SumsURL string `json:"-"`
}

type apiAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type apiRelease struct {
	Tag    string     `json:"tag_name"`
	URL    string     `json:"html_url"`
	Body   string     `json:"body"`
	Draft  bool       `json:"draft"`
	Assets []apiAsset `json:"assets"`
}

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)`)

func parse(version string) ([3]int, bool) {
	match := versionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if match == nil {
		return [3]int{}, false
	}
	var parsed [3]int
	for index := range parsed {
		value, err := strconv.Atoi(match[index+1])
		if err != nil {
			return [3]int{}, false
		}
		parsed[index] = value
	}
	return parsed, true
}

// Comparable reports whether version is a release version (not "dev").
func Comparable(version string) bool {
	_, ok := parse(version)
	return ok
}

// Newer reports whether tag is a strictly higher version than current.
// Prerelease suffixes are ignored; unparsable versions (such as "dev") never
// yield a notice.
func Newer(current, tag string) bool {
	have, ok := parse(current)
	if !ok {
		return false
	}
	want, ok := parse(tag)
	if !ok {
		return false
	}
	return greater(want, have)
}

func greater(a, b [3]int) bool {
	for index := range a {
		if a[index] != b[index] {
			return a[index] > b[index]
		}
	}
	return false
}

// Latest returns the highest-versioned non-draft release from the API.
// ok is false when there are no usable releases.
func Latest(ctx context.Context, client *http.Client, apiURL string) (release Release, ok bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return Release{}, false, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "TauShell-update-check")
	response, err := client.Do(request)
	if err != nil {
		return Release{}, false, fmt.Errorf("query releases: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, false, fmt.Errorf("query releases: HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody))
	if err != nil {
		return Release{}, false, fmt.Errorf("read releases: %w", err)
	}
	var releases []apiRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return Release{}, false, fmt.Errorf("parse releases: %w", err)
	}
	var best [3]int
	for _, candidate := range releases {
		version, parsed := parse(candidate.Tag)
		if candidate.Draft || !parsed || !strings.HasPrefix(candidate.URL, ReleasesPrefix) {
			continue
		}
		if !ok || greater(version, best) {
			best, release, ok = version, newRelease(candidate), true
		}
	}
	return release, ok, nil
}

// newRelease keeps asset URLs only when they point at this repository's release downloads.
func newRelease(candidate apiRelease) Release {
	release := Release{Tag: candidate.Tag, URL: candidate.URL, Notes: candidate.Body}
	zipName := fmt.Sprintf("TauShell-%s-windows-x64.zip", strings.TrimPrefix(candidate.Tag, "v"))
	for _, asset := range candidate.Assets {
		if !strings.HasPrefix(asset.URL, DownloadPrefix) {
			continue
		}
		switch asset.Name {
		case zipName:
			release.ZipURL = asset.URL
		case SumsName:
			release.SumsURL = asset.URL
		}
	}
	return release
}
