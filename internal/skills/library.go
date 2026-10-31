package skills

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ma99us/tau-shell/internal/presets"

	"github.com/ma99us/tau-shell/internal/fsutil"
)

//go:embed all:bundled
var bundledFiles embed.FS

// Bundled returns the vendored snapshot embedded in the executable.
func Bundled() fs.FS {
	sub, err := fs.Sub(bundledFiles, "bundled")
	if err != nil {
		return emptyFS{}
	}
	return sub
}

type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

// Config is the resolved skills configuration.
type Config struct {
	Enabled      bool
	Repo         string // "" disables fetching
	Ref          string
	AutoUpdate   bool
	LocalSources []string
}

// DefaultConfig is used when config.yaml has no skills block.
func DefaultConfig() Config {
	return Config{Enabled: true, Repo: DefaultRepo, Ref: DefaultRef}
}

type state struct {
	Applied  string   `json:"applied,omitempty"`
	Disabled []string `json:"disabled,omitempty"`
}

// Library resolves skills for sessions and keeps the GitHub source current.
type Library struct {
	dir      string
	config   Config
	remote   *Remote
	bundled  fs.FS
	personal map[string][]string

	mu       sync.Mutex
	state    state
	latest   string
	etag     string
	lastErr  string
	checkedA time.Time
}

// NewLibrary opens the library stored under dir (normally ~/.tau_shell).
// personal maps a harness id to that harness's personal skill folders.
func NewLibrary(dir string, config Config, personal map[string][]string) (*Library, error) {
	library := &Library{dir: dir, config: config, bundled: Bundled(), personal: personal}
	if config.Repo != "" {
		remote, err := NewRemote(config.Repo, config.Ref)
		if err != nil {
			return nil, err
		}
		library.remote = remote
	}
	library.loadState()
	return library, nil
}

func (l *Library) statePath() string { return filepath.Join(l.dir, "skills-state.json") }

func (l *Library) loadState() {
	data, err := os.ReadFile(l.statePath())
	if err == nil {
		_ = json.Unmarshal(data, &l.state)
	}
}

func (l *Library) saveStateLocked() error {
	data, err := json.Marshal(l.state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.dir, 0o700); err != nil {
		return err
	}
	return fsutil.ReplaceFile(l.statePath(), data)
}

func (l *Library) snapshotDir(sha string) string {
	return filepath.Join(l.dir, "sources", l.remote.Key(), sha)
}

// bundledSHA reads the commit recorded in the bundled SOURCE file.
func (l *Library) bundledSHA() string {
	data, err := fs.ReadFile(l.bundled, "SOURCE")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "commit:"); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (l *Library) appliedSHALocked() string {
	if l.remote != nil && l.state.Applied != "" {
		if _, err := os.Stat(l.snapshotDir(l.state.Applied)); err == nil {
			return l.state.Applied
		}
	}
	return ""
}

// sources lists skill sources by precedence, highest first.
func (l *Library) sources() []Loaded {
	var sources []Loaded
	for _, dir := range l.config.LocalSources {
		if _, err := os.Stat(dir); err != nil {
			sources = append(sources, Loaded{Kind: SourceLocal, Label: dir, Warnings: []string{fmt.Sprintf("local skill source %s is missing", dir)}})
			continue
		}
		sources = append(sources, Load(DirFS(dir), SourceLocal, dir))
	}
	l.mu.Lock()
	applied := l.appliedSHALocked()
	l.mu.Unlock()
	if applied != "" {
		sources = append(sources, Load(DirFS(l.snapshotDir(applied)), SourceCookbook, "cookbook "+short(applied)))
	}
	sources = append(sources, Load(l.bundled, SourceBundled, "bundled "+short(l.bundledSHA())))
	return sources
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// Entry is one skill as the UI shows it for a harness.
type Entry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Enabled     bool   `json:"enabled"`
	// Personal means the user's own copy is used instead of TauShell's.
	Personal bool `json:"personal"`
}

// Status describes the library for the UI.
type Status struct {
	Enabled         bool     `json:"enabled"`
	Repo            string   `json:"repo,omitempty"`
	Ref             string   `json:"ref,omitempty"`
	Source          string   `json:"source"`
	Applied         string   `json:"applied,omitempty"`
	Latest          string   `json:"latest,omitempty"`
	UpdateAvailable bool     `json:"updateAvailable"`
	Message         string   `json:"message,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

func (l *Library) disabledSet() map[string]bool {
	set := map[string]bool{}
	for _, name := range l.state.Disabled {
		set[name] = true
	}
	return set
}

// Status reports the active source and any pending update.
func (l *Library) Status() Status {
	sources := l.sources()
	l.mu.Lock()
	defer l.mu.Unlock()
	status := Status{Enabled: l.config.Enabled, Message: l.lastErr}
	if l.remote != nil {
		status.Repo = l.remote.Owner + "/" + l.remote.Repo
		status.Ref = l.remote.Ref
	}
	if applied := l.appliedSHALocked(); applied != "" {
		status.Source, status.Applied = SourceCookbook, short(applied)
	} else {
		status.Source, status.Applied = SourceBundled, short(l.bundledSHA())
	}
	current := l.appliedSHALocked()
	if current == "" {
		current = l.bundledSHA()
	}
	if l.latest != "" && l.latest != current {
		status.UpdateAvailable = true
		status.Latest = short(l.latest)
	}
	for _, source := range sources {
		status.Warnings = append(status.Warnings, source.Warnings...)
	}
	return status
}

// List returns every skill TauShell offers for the harness, with the user's
// own personal copies marked.
func (l *Library) List(harness string) []Entry {
	sources := l.sources()
	personal := PersonalNames(l.personal[harness])
	l.mu.Lock()
	disabled := l.disabledSet()
	l.mu.Unlock()
	merged := Merge(sources)
	entries := make([]Entry, 0, len(merged))
	for _, skill := range merged {
		entries = append(entries, Entry{
			Name: skill.Name, Description: skill.Description, Source: skill.Source,
			Enabled: !disabled[skill.Name], Personal: personal[skill.Name],
		})
	}
	return entries
}

// SetEnabled turns one skill on or off for every session launched after.
func (l *Library) SetEnabled(name string, enabled bool) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid skill name %q", name)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	set := l.disabledSet()
	if enabled {
		delete(set, name)
	} else {
		set[name] = true
	}
	l.state.Disabled = l.state.Disabled[:0]
	for item := range set {
		l.state.Disabled = append(l.state.Disabled, item)
	}
	sort.Strings(l.state.Disabled)
	return l.saveStateLocked()
}

// Available returns the skill names a session on this harness can use: the
// enabled TauShell skills plus the user's own personal skills.
func (l *Library) Available(harness string) map[string]bool {
	names := PersonalNames(l.personal[harness])
	if !l.config.Enabled {
		return names
	}
	for _, entry := range l.List(harness) {
		if entry.Enabled {
			names[entry.Name] = true
		}
	}
	return names
}

// Presets returns source presets, with higher-precedence sources replacing
// same-named presets of lower ones.
func (l *Library) Presets() []presets.Preset {
	if !l.config.Enabled {
		return nil
	}
	var merged []presets.Preset
	for _, source := range l.sources() {
		merged = presets.Merge(source.Presets, merged)
	}
	return merged
}

// Prepare materializes the enabled skills for a harness and returns the
// plugin folder, or "" when there is nothing to load. Skills the user already
// has personally are left out so their copy wins.
func (l *Library) Prepare(harness string) (dir string, names []string, warnings []string, err error) {
	if !l.config.Enabled {
		return "", nil, nil, nil
	}
	personal := PersonalNames(l.personal[harness])
	l.mu.Lock()
	disabled := l.disabledSet()
	l.mu.Unlock()
	var chosen []Skill
	for _, skill := range Merge(l.sources()) {
		if disabled[skill.Name] || personal[skill.Name] {
			continue
		}
		chosen = append(chosen, skill)
		names = append(names, skill.Name)
	}
	dir, warnings, err = Materialize(filepath.Join(l.dir, "plugins", harness), chosen)
	return dir, names, warnings, err
}

// GC removes stale plugin folders.
func (l *Library) GC() {
	entries, _ := os.ReadDir(filepath.Join(l.dir, "plugins"))
	for _, entry := range entries {
		GC(filepath.Join(l.dir, "plugins", entry.Name()), 30*24*time.Hour)
	}
}

// Check asks GitHub for the newest commit. A newer commit is only recorded
// unless auto-update is on, in which case it is applied. Failures keep the
// last applied snapshot and are reported through Status.
func (l *Library) Check(ctx context.Context) error {
	if l.remote == nil || !l.config.Enabled {
		return nil
	}
	l.mu.Lock()
	etag := l.etag
	l.mu.Unlock()
	sha, newETag, changed, err := l.remote.Latest(ctx, etag)
	l.mu.Lock()
	l.checkedA = time.Now()
	if err != nil {
		l.lastErr = "Cookbook update check failed: " + err.Error()
		l.mu.Unlock()
		return err
	}
	l.lastErr = ""
	l.etag = newETag
	if changed {
		l.latest = sha
	}
	l.mu.Unlock()
	if l.config.AutoUpdate {
		return l.Apply(ctx)
	}
	return nil
}

// Apply fetches the latest known commit and makes it the active snapshot.
func (l *Library) Apply(ctx context.Context) error {
	if l.remote == nil {
		return errors.New("skill fetching is disabled (skills.repo is empty)")
	}
	l.mu.Lock()
	sha := l.latest
	current := l.appliedSHALocked()
	if current == "" {
		current = l.bundledSHA()
	}
	l.mu.Unlock()
	if sha == "" || sha == current {
		return nil
	}
	dest := l.snapshotDir(sha)
	if _, err := os.Stat(dest); err != nil {
		if err := l.remote.Fetch(ctx, sha, dest); err != nil {
			l.mu.Lock()
			l.lastErr = "Cookbook update failed: " + err.Error()
			l.mu.Unlock()
			return err
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state.Applied = sha
	l.lastErr = ""
	return l.saveStateLocked()
}
