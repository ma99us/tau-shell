package mei

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ma99us/tau-shell/internal/fsutil"
)

func fixturePage(rows int) string {
	var page strings.Builder
	page.WriteString(`<html><table aria-label="Model rankings"><thead><tr><th>#</th></tr></thead><tbody><!--rows-->`)
	for index := 1; index <= rows; index++ {
		badge := ""
		if index == 1 {
			badge = `<span class="badge" tabindex="0" aria-label="Confident score"><svg></svg></span>`
		}
		if index == 2 {
			badge = `<span class="badge" aria-label="Rough estimate"></span>`
		}
		fmt.Fprintf(&page, `<tr><td class="rank" data-value="%d">%d</td><td class="model" data-value="Model &amp; %d"><a class="model-link" href="./vendor/m-%d" data-model-link="id-%d">Model %d</a>%s <a class="lab lab-link" href="./vendor">Vendor</a></td><td class="bar" data-value="1"></td><td class="num" data-value="%d.5">x</td><td class="num" data-value="1.25">x</td><td class="num" data-value="0.5">x</td></tr>`,
			index, index, index, index, index, index, badge, 80-index)
	}
	page.WriteString(`</tbody></table></html>`)
	return page.String()
}

func TestParseReadsRowsAndBadges(t *testing.T) {
	entries, err := Parse(fixturePage(12))
	if err != nil || len(entries) != 12 {
		t.Fatalf("Parse = %d entries, %v", len(entries), err)
	}
	first := entries[0]
	if first.Rank != 1 || first.Name != "Model & 1" || first.Vendor != "vendor" || first.Slug != "m-1" || first.ID != "id-1" ||
		first.Score != 79.5 || first.Cost != 1.25 || first.Tokens != 0.5 || first.Confidence != "confident" {
		t.Fatalf("first = %+v", first)
	}
	if entries[1].Confidence != "rough" || entries[2].Confidence != "" {
		t.Fatalf("confidence = %q %q", entries[1].Confidence, entries[2].Confidence)
	}
}

func TestParseRejectsBadPages(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"error page":       "<html><h1>404</h1></html>",
		"changed layout":   strings.ReplaceAll(fixturePage(12), `class="rank"`, `class="position"`),
		"too few rows":     fixturePage(3),
		"truncated":        fixturePage(12)[:600],
		"implausible":      strings.Replace(fixturePage(12), `data-value="78.5"`, `data-value="9999"`, 1),
		"duplicate id":     strings.Replace(fixturePage(12), `data-model-link="id-2"`, `data-model-link="id-1"`, 1),
		"non numeric cell": strings.Replace(fixturePage(12), `data-value="78.5"`, `data-value="abc"`, 1),
	}
	for name, page := range cases {
		if _, err := Parse(page); err == nil {
			t.Errorf("%s: Parse succeeded", name)
		}
	}
}

func TestEmbeddedSnapshotIsValidOrEmpty(t *testing.T) {
	if _, ok := parseFile(snapshotJSON); !ok {
		t.Fatal("embedded snapshot.json is invalid; regenerate with MEI_UPDATE_SNAPSHOT=1 go test ./internal/mei -run TestGenerateSnapshot")
	}
}

// TestGenerateSnapshot refreshes snapshot.json from the live site. Opt in with
// MEI_UPDATE_SNAPSHOT=1; it is skipped otherwise.
func TestGenerateSnapshot(t *testing.T) {
	if os.Getenv("MEI_UPDATE_SNAPSHOT") != "1" {
		t.Skip("set MEI_UPDATE_SNAPSHOT=1 to refresh snapshot.json from the live site")
	}
	entries, err := Fetcher{}.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(fileData{FetchedAt: time.Now().UTC().Truncate(time.Second), Entries: entries}, "", "  ")
	if err := fsutil.ReplaceFile("snapshot.json", append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}

func testStore(t *testing.T, handler http.HandlerFunc) (*Store, *httptest.Server, string) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	path := filepath.Join(t.TempDir(), "mei.json")
	return NewStore(path, Fetcher{URL: server.URL, Client: server.Client()}), server, path
}

func TestRefreshSuccessCachesAndSurvivesRestart(t *testing.T) {
	store, server, path := testStore(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, fixturePage(12)) })
	if err := store.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	data := store.Data()
	if data.Source != SourceCache || len(data.Entries) != 12 || data.LastError != "" || data.Stale {
		t.Fatalf("data = %+v", data)
	}
	reloaded := NewStore(path, Fetcher{URL: server.URL, Client: server.Client()})
	if got := reloaded.Data(); got.Source != SourceCache || len(got.Entries) != 12 {
		t.Fatalf("reloaded = %+v", got)
	}
	if reloaded.NeedsRefresh() {
		t.Fatal("fresh cache should not need a refresh")
	}
}

func TestFailuresKeepGoodDataAndBackOff(t *testing.T) {
	var mode atomic.Value
	mode.Store("ok")
	store, _, _ := testStore(t, func(w http.ResponseWriter, r *http.Request) {
		switch mode.Load() {
		case "500":
			http.Error(w, "boom", http.StatusInternalServerError)
		case "404":
			http.NotFound(w, r)
		case "garbage":
			fmt.Fprint(w, "<html>new design</html>")
		case "huge":
			fmt.Fprint(w, strings.Repeat("x", maxBodyBytes+10))
		case "slow":
			<-r.Context().Done()
		default:
			fmt.Fprint(w, fixturePage(12))
		}
	})
	clock := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return clock }
	if err := store.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, failing := range []string{"500", "404", "garbage", "huge"} {
		mode.Store(failing)
		if err := store.Refresh(context.Background()); err == nil {
			t.Fatalf("%s: expected error", failing)
		}
		data := store.Data()
		if len(data.Entries) != 12 || data.Source != SourceCache || data.LastError == "" {
			t.Fatalf("%s: data = %+v", failing, data)
		}
		if store.NeedsRefresh() {
			t.Fatalf("%s: refresh should be backed off", failing)
		}
	}
	clock = clock.Add(25 * time.Hour)
	if !store.NeedsRefresh() {
		t.Fatal("refresh should be due after the backoff")
	}
	if data := store.Data(); data.Stale {
		t.Fatalf("25h old data should not be flagged stale yet: %+v", data)
	}
	mode.Store("ok")
	if err := store.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if data := store.Data(); data.LastError != "" || store.failures != 0 {
		t.Fatalf("recovery did not reset state: %+v", data)
	}
	clock = clock.Add(80 * time.Hour)
	if !store.Data().Stale {
		t.Fatal("80h old data should be flagged stale")
	}
}

func TestBackoffSchedule(t *testing.T) {
	want := []time.Duration{time.Hour, 3 * time.Hour, 12 * time.Hour, 24 * time.Hour, 24 * time.Hour}
	for index, expected := range want {
		if got := backoff(index + 1); got != expected {
			t.Errorf("backoff(%d) = %v, want %v", index+1, got, expected)
		}
	}
}

func TestRefreshCancelledContextAndTimeout(t *testing.T) {
	store, _, _ := testStore(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := store.Refresh(ctx); err == nil {
		t.Fatal("expected a timeout error")
	}
	if got := store.Data(); got.LastError == "" {
		t.Fatal("failure was not recorded")
	}
}

func TestRedirectToAnotherHostIsRefused(t *testing.T) {
	store, _, _ := testStore(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.invalid/mei", http.StatusFound)
	})
	err := store.Refresh(context.Background())
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlainHTTPIsRefused(t *testing.T) {
	if _, err := (Fetcher{URL: "http://git-aarvin.github.io/mei/"}).Fetch(context.Background()); err == nil {
		t.Fatal("http fetch should be refused")
	}
}

func TestCorruptCacheFallsBackToSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mei.json")
	for _, content := range []string{"{not json", `{"entries":[{"id":"x"}]}`, ""} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		store := NewStore(path, Fetcher{})
		data := store.Data()
		if data.Source == SourceCache {
			t.Fatalf("corrupt cache %q was accepted", content)
		}
		if !store.NeedsRefresh() {
			t.Fatal("a missing cache should need a refresh")
		}
	}
}

func TestOfflineWithNoCacheStillServes(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "mei.json"), Fetcher{URL: "https://127.0.0.1:1/", Client: &http.Client{}})
	if err := store.Refresh(context.Background()); err == nil {
		t.Fatal("expected a connection error")
	}
	data := store.Data()
	if data.Source != SourceSnapshot && data.Source != SourceNone {
		t.Fatalf("source = %q", data.Source)
	}
	if data.LastError == "" {
		t.Fatal("last error missing")
	}
}
