package ide

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// UninstallEntry is one Windows "installed programs" record.
type UninstallEntry struct {
	DisplayName     string
	InstallLocation string
}

// uninstallEntries is replaced with the registry reader on Windows.
var uninstallEntries = func() []UninstallEntry { return nil }

var envReference = regexp.MustCompile(`%([^%]+)%`)

// expandEnv resolves %VAR% references and reports false when one is unset.
func expandEnv(template string) (string, bool) {
	ok := true
	out := envReference.ReplaceAllStringFunc(template, func(ref string) string {
		value := os.Getenv(ref[1 : len(ref)-1])
		if value == "" {
			ok = false
		}
		return value
	})
	return out, ok
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Locate finds the executable of one catalog IDE, or "" when it is not installed.
func Locate(def Def) string {
	for _, template := range def.Exes {
		pattern, ok := expandEnv(template)
		if !ok {
			continue
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		sort.Strings(matches)
		for i := len(matches) - 1; i >= 0; i-- {
			if fileExists(matches[i]) {
				return matches[i]
			}
		}
	}
	if def.UninstallExe != "" && len(def.Uninstall) > 0 {
		for _, entry := range uninstallEntries() {
			if entry.InstallLocation == "" {
				continue
			}
			for _, prefix := range def.Uninstall {
				if !strings.HasPrefix(entry.DisplayName, prefix) {
					continue
				}
				candidate := filepath.Join(strings.Trim(entry.InstallLocation, `"`), def.UninstallExe)
				if fileExists(candidate) {
					return candidate
				}
			}
		}
	}
	if def.PathExe != "" {
		if path, err := exec.LookPath(def.PathExe); err == nil {
			return path
		}
	}
	return ""
}

const detectTTL = time.Minute

var detectCache struct {
	sync.Mutex
	at    time.Time
	items []Installed
}

// DetectInstalled returns the catalog IDEs present on this machine. The result
// is cached for a minute because the check walks install folders and the registry.
func DetectInstalled() []Installed {
	detectCache.Lock()
	defer detectCache.Unlock()
	if detectCache.items != nil && time.Since(detectCache.at) < detectTTL {
		return detectCache.items
	}
	items := []Installed{}
	for _, def := range Catalog {
		if exe := Locate(def); exe != "" {
			items = append(items, Installed{Def: def, Exe: exe})
		}
	}
	detectCache.items, detectCache.at = items, time.Now()
	return items
}

// ResetCache forgets the cached detection result.
func ResetCache() {
	detectCache.Lock()
	detectCache.items = nil
	detectCache.Unlock()
}
