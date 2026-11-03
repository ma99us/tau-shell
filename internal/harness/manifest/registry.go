package manifest

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

//go:embed builtin/*.yaml
var builtinFiles embed.FS

const maxManifestBytes = 64 << 10

// LoadError is a manifest file that could not be used.
type LoadError struct {
	Path  string `json:"path"`
	Error string `json:"error"`
}

// Registry is the set of harnesses known for this run.
type Registry struct {
	items  []*Manifest
	byID   map[string]*Manifest
	errors []LoadError
}

var (
	defaultMu       sync.RWMutex
	defaultRegistry *Registry
)

// Builtins parses the embedded manifests. They are covered by tests, so a
// failure here is a build defect and panics at startup rather than hiding.
func Builtins() *Registry {
	registry := &Registry{byID: map[string]*Manifest{}}
	entries, err := builtinFiles.ReadDir("builtin")
	if err != nil {
		panic(fmt.Sprintf("read built-in harness manifests: %v", err))
	}
	for _, entry := range entries {
		data, err := builtinFiles.ReadFile("builtin/" + entry.Name())
		if err != nil {
			panic(fmt.Sprintf("read built-in harness manifest %s: %v", entry.Name(), err))
		}
		item, err := Decode(data, true)
		if err != nil {
			panic(fmt.Sprintf("invalid built-in harness manifest %s: %v", entry.Name(), err))
		}
		item.Path = "builtin/" + entry.Name()
		if _, duplicate := registry.byID[item.ID]; duplicate {
			panic(fmt.Sprintf("duplicate built-in harness %q", item.ID))
		}
		registry.byID[item.ID] = item
		registry.items = append(registry.items, item)
	}
	sortItems(registry.items)
	return registry
}

func sortItems(items []*Manifest) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Order != items[j].Order {
			return items[i].Order < items[j].Order
		}
		return items[i].ID < items[j].ID
	})
}

// Load returns the built-in harnesses plus every valid manifest in userDir.
// Built-in IDs are reserved. A bad file is reported in Errors and skipped; it
// never prevents TauShell from starting.
func Load(userDir string) *Registry {
	registry := Builtins()
	if userDir == "" {
		return registry
	}
	entries, err := os.ReadDir(userDir)
	if err != nil {
		if !os.IsNotExist(err) {
			registry.errors = append(registry.errors, LoadError{Path: userDir, Error: err.Error()})
		}
		return registry
	}
	var users []*Manifest
	for _, entry := range entries {
		name := entry.Name()
		extension := strings.ToLower(filepath.Ext(name))
		if entry.IsDir() || (extension != ".yaml" && extension != ".yml") {
			continue
		}
		path := filepath.Join(userDir, name)
		item, err := loadUserFile(path)
		if err == nil {
			if _, taken := registry.byID[item.ID]; taken {
				if existing := registry.byID[item.ID]; existing.Builtin {
					err = fmt.Errorf("id: %q is a built-in harness and cannot be redefined; copy it under a new id", item.ID)
				} else {
					err = fmt.Errorf("id: %q is already defined by %s", item.ID, filepath.Base(existing.Path))
				}
			}
		}
		if err != nil {
			registry.errors = append(registry.errors, LoadError{Path: path, Error: err.Error()})
			continue
		}
		registry.byID[item.ID] = item
		users = append(users, item)
	}
	sortItems(users)
	registry.items = append(registry.items, users...)
	return registry
}

func loadUserFile(path string) (*Manifest, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxManifestBytes {
		return nil, fmt.Errorf("the file is larger than %d KB", maxManifestBytes>>10)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	item, err := Decode(data, false)
	if err != nil {
		return nil, err
	}
	item.Path = path
	return item, nil
}

// All lists harnesses in display order: built-ins, then user manifests.
func (registry *Registry) All() []*Manifest {
	return append([]*Manifest(nil), registry.items...)
}

func (registry *Registry) Get(id string) (*Manifest, bool) {
	item, ok := registry.byID[id]
	return item, ok
}

// Errors lists manifest files that were skipped.
func (registry *Registry) Errors() []LoadError {
	return append([]LoadError(nil), registry.errors...)
}

// SetDefault installs the registry used by the package-level helpers.
func SetDefault(registry *Registry) {
	defaultMu.Lock()
	defaultRegistry = registry
	defaultMu.Unlock()
}

// Default returns the installed registry, or the built-ins when none was set.
func Default() *Registry {
	defaultMu.RLock()
	registry := defaultRegistry
	defaultMu.RUnlock()
	if registry != nil {
		return registry
	}
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultRegistry == nil {
		defaultRegistry = Builtins()
	}
	return defaultRegistry
}

// Get looks a harness up in the default registry.
func Get(id string) (*Manifest, bool) { return Default().Get(id) }

// All lists the default registry's harnesses.
func All() []*Manifest { return Default().All() }
