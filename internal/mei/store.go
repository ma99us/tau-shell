package mei

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/ma99us/tau-shell/internal/fsutil"
)

const (
	// URL is the only address ever fetched.
	URL             = "https://git-aarvin.github.io/mei/"
	RefreshInterval = 24 * time.Hour
	staleAfter      = 3 * RefreshInterval
	fetchTimeout    = 10 * time.Second
	maxBodyBytes    = 2 << 20
	maxRedirects    = 3
)

// Data sources, from best to worst.
const (
	SourceCache    = "cache"
	SourceSnapshot = "snapshot"
	SourceNone     = "none"
)

//go:embed snapshot.json
var snapshotJSON []byte

type fileData struct {
	FetchedAt time.Time `json:"fetched_at"`
	Entries   []Entry   `json:"entries"`
}

// Data is what consumers see: the best ranking available right now.
type Data struct {
	Entries   []Entry
	FetchedAt time.Time
	Source    string
	// Stale is true when the data is older than three refresh intervals.
	Stale bool
	// LastError describes the latest failed refresh and is empty after a success.
	LastError string
}

// Fetcher downloads and parses the ranking page.
type Fetcher struct {
	URL    string
	Client *http.Client
}

// Fetch returns a validated ranking or an error; never a partial table.
func (fetcher Fetcher) Fetch(ctx context.Context) ([]Entry, error) {
	target := fetcher.URL
	if target == "" {
		target = URL
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" {
		return nil, errors.New("mei: only https is allowed")
	}
	client := fetcher.Client
	if client == nil {
		client = &http.Client{}
	}
	copied := *client
	copied.Timeout = fetchTimeout
	copied.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return errors.New("too many redirects")
		}
		if request.URL.Scheme != "https" || request.URL.Host != parsed.Host {
			return fmt.Errorf("redirect to %s refused", request.URL.Host)
		}
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/html")
	request.Header.Set("User-Agent", "TauShell")
	response, err := copied.Do(request)
	if err != nil {
		return nil, fmt.Errorf("mei: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mei: HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("mei: read response: %w", err)
	}
	if len(body) > maxBodyBytes {
		return nil, fmt.Errorf("mei: response exceeds %d bytes", maxBodyBytes)
	}
	entries, err := Parse(string(body))
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// Store serves the ranking and refreshes it in the background. Failed
// refreshes keep the previous data and back off (1h, 3h, 12h, then daily).
type Store struct {
	path    string
	fetcher Fetcher
	now     func() time.Time

	mu        sync.Mutex
	refresh   sync.Mutex
	data      fileData
	source    string
	lastError string
	failures  int
	nextTry   time.Time
}

// NewStore loads the cache at path, falling back to the embedded snapshot. A
// missing or corrupt cache is ignored.
func NewStore(path string, fetcher Fetcher) *Store {
	store := &Store{path: path, fetcher: fetcher, now: time.Now, source: SourceNone}
	if loaded, ok := readFile(path); ok {
		store.data, store.source = loaded, SourceCache
	} else if snapshot, ok := parseFile(snapshotJSON); ok {
		store.data, store.source = snapshot, SourceSnapshot
	}
	return store
}

func parseFile(data []byte) (fileData, bool) {
	var loaded fileData
	if json.Unmarshal(data, &loaded) != nil || Validate(loaded.Entries) != nil {
		return fileData{}, false
	}
	return loaded, true
}

func readFile(path string) (fileData, bool) {
	if path == "" {
		return fileData{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileData{}, false
	}
	return parseFile(data)
}

// Data returns the best ranking available. It never blocks on the network.
func (store *Store) Data() Data {
	store.mu.Lock()
	defer store.mu.Unlock()
	return Data{
		Entries:   store.data.Entries,
		FetchedAt: store.data.FetchedAt,
		Source:    store.source,
		Stale:     store.source != SourceNone && store.now().Sub(store.data.FetchedAt) > staleAfter,
		LastError: store.lastError,
	}
}

// NeedsRefresh reports whether a background refresh is due, honoring the
// failure backoff.
func (store *Store) NeedsRefresh() bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	now := store.now()
	if store.failures > 0 {
		return !now.Before(store.nextTry)
	}
	return store.source != SourceCache || now.Sub(store.data.FetchedAt) >= RefreshInterval
}

// Refresh fetches the ranking once. Concurrent calls collapse into one. On any
// failure the previous data stays in place and the error is returned.
func (store *Store) Refresh(ctx context.Context) error {
	if !store.refresh.TryLock() {
		return errors.New("mei: refresh already running")
	}
	defer store.refresh.Unlock()
	entries, err := store.fetcher.Fetch(ctx)
	if err == nil {
		fresh := fileData{FetchedAt: store.now().UTC(), Entries: entries}
		var encoded []byte
		if encoded, err = json.MarshalIndent(fresh, "", "  "); err == nil && store.path != "" {
			if err = fsutil.ReplaceFile(store.path, encoded); err != nil {
				err = fmt.Errorf("mei: save cache: %w", err)
			}
		}
		if err == nil {
			store.mu.Lock()
			recovered := store.failures > 0
			store.data, store.source, store.lastError, store.failures = fresh, SourceCache, "", 0
			store.mu.Unlock()
			if recovered {
				diagnostics.Info("MEI ranking refreshed after earlier failures")
			}
			return nil
		}
	}
	store.mu.Lock()
	store.failures++
	store.lastError = err.Error()
	store.nextTry = store.now().Add(backoff(store.failures))
	first := store.failures == 1
	store.mu.Unlock()
	if first {
		diagnostics.Warn("MEI ranking refresh failed; using previous data", err)
	}
	return err
}

func backoff(failures int) time.Duration {
	switch failures {
	case 1:
		return time.Hour
	case 2:
		return 3 * time.Hour
	case 3:
		return 12 * time.Hour
	}
	return RefreshInterval
}
