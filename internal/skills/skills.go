// Package skills assembles the Agent Skills TauShell offers to harness
// sessions. Skills come from several sources (a local checkout, a fetched
// GitHub repository, a bundled snapshot), are validated, merged by
// precedence, and materialized into an immutable per-harness plugin folder
// that each harness loads with --plugin-dir.
package skills

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ma99us/tau-shell/internal/presets"
	"gopkg.in/yaml.v3"
)

const (
	ManifestName = "tau-shell.yaml"
	PluginName   = "taushell"

	SourcePersonal = "personal (yours)"
	SourceLocal    = "local"
	SourceCookbook = "cookbook"
	SourceBundled  = "bundled"

	maxSkillFileBytes = 1 << 20
	maxSkillBytes     = 8 << 20
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Manifest is the optional tau-shell.yaml at a source root.
type Manifest struct {
	Version int              `yaml:"version"`
	Skills  []string         `yaml:"skills"`
	Presets []presets.Preset `yaml:"presets"`
}

// Skill is one validated skill folder inside a source.
type Skill struct {
	Name        string
	Description string
	Source      string
	root        fs.FS
	dir         string
}

// Loaded is the content of one source: its skills and presets.
type Loaded struct {
	Kind     string
	Label    string
	Skills   []Skill
	Presets  []presets.Preset
	Warnings []string
}

// ParseManifest reads and validates tau-shell.yaml. A missing file is not an
// error: ok is false and every skill in the source is exposed.
func ParseManifest(root fs.FS) (manifest Manifest, ok bool, err error) {
	data, err := fs.ReadFile(root, ManifestName)
	if errors.Is(err, fs.ErrNotExist) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil && !errors.Is(err, io.EOF) {
		return Manifest{}, false, fmt.Errorf("parse %s: %w", ManifestName, err)
	}
	if manifest.Version != 0 && manifest.Version != 1 {
		return Manifest{}, false, fmt.Errorf("%s: unsupported version %d", ManifestName, manifest.Version)
	}
	for _, name := range manifest.Skills {
		if !namePattern.MatchString(name) {
			return Manifest{}, false, fmt.Errorf("%s: invalid skill name %q", ManifestName, name)
		}
	}
	if err := presets.Validate(manifest.Presets); err != nil {
		return Manifest{}, false, fmt.Errorf("%s: %w", ManifestName, err)
	}
	return manifest, true, nil
}

// Load scans a source root. Only skills named by the manifest are exposed
// when one exists. Invalid skills are skipped with a warning, never silently.
func Load(root fs.FS, kind, label string) Loaded {
	loaded := Loaded{Kind: kind, Label: label}
	manifest, hasManifest, err := ParseManifest(root)
	if err != nil {
		loaded.Warnings = append(loaded.Warnings, fmt.Sprintf("%s: %v", label, err))
		return loaded
	}
	loaded.Presets = manifest.Presets
	entries, err := fs.ReadDir(root, "skills")
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			loaded.Warnings = append(loaded.Warnings, fmt.Sprintf("%s: read skills: %v", label, err))
		}
		return loaded
	}
	wanted := map[string]bool{}
	for _, name := range manifest.Skills {
		wanted[name] = true
	}
	limited := hasManifest && len(manifest.Skills) > 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if limited && !wanted[name] {
			continue
		}
		skill, err := readSkill(root, name)
		if err != nil {
			loaded.Warnings = append(loaded.Warnings, fmt.Sprintf("%s: skipped skill %q: %v", label, name, err))
			continue
		}
		skill.Source = kind
		loaded.Skills = append(loaded.Skills, skill)
	}
	if limited {
		for name := range wanted {
			found := false
			for _, skill := range loaded.Skills {
				found = found || skill.Name == name
			}
			if !found {
				loaded.Warnings = append(loaded.Warnings, fmt.Sprintf("%s: manifest skill %q is missing or invalid", label, name))
			}
		}
	}
	sort.Slice(loaded.Skills, func(i, j int) bool { return loaded.Skills[i].Name < loaded.Skills[j].Name })
	return loaded
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func readSkill(root fs.FS, folder string) (Skill, error) {
	data, err := fs.ReadFile(root, path.Join("skills", folder, "SKILL.md"))
	if err != nil {
		return Skill{}, errors.New("no SKILL.md")
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return Skill{}, errors.New("SKILL.md has no frontmatter")
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return Skill{}, errors.New("SKILL.md frontmatter is not closed")
	}
	var meta frontmatter
	if err := yaml.Unmarshal([]byte(text[4:4+end]), &meta); err != nil {
		return Skill{}, fmt.Errorf("frontmatter: %w", err)
	}
	if meta.Name != folder {
		return Skill{}, fmt.Errorf("frontmatter name %q does not match folder", meta.Name)
	}
	if !namePattern.MatchString(meta.Name) {
		return Skill{}, errors.New("invalid skill name")
	}
	if strings.TrimSpace(meta.Description) == "" {
		return Skill{}, errors.New("missing description")
	}
	return Skill{Name: meta.Name, Description: strings.TrimSpace(meta.Description), root: root, dir: path.Join("skills", folder)}, nil
}

// DirFS opens a disk folder as a source root.
func DirFS(dir string) fs.FS { return os.DirFS(dir) }

// PersonalNames lists skill folder names found in the user's own skill
// folders; those win over TauShell's copies.
func PersonalNames(dirs []string) map[string]bool {
	names := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if _, err := os.Stat(filepath.Join(dir, entry.Name(), "SKILL.md")); err == nil {
				names[entry.Name()] = true
			}
		}
	}
	return names
}

// Merge picks one skill per name from sources ordered by precedence, highest
// first. It returns the winners sorted by name.
func Merge(sources []Loaded) []Skill {
	chosen := map[string]Skill{}
	for _, source := range sources {
		for _, skill := range source.Skills {
			if _, taken := chosen[skill.Name]; !taken {
				chosen[skill.Name] = skill
			}
		}
	}
	merged := make([]Skill, 0, len(chosen))
	for _, skill := range chosen {
		merged = append(merged, skill)
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Name < merged[j].Name })
	return merged
}

type pluginFile struct {
	rel  string
	data []byte
}

func collectFiles(skill Skill) ([]pluginFile, error) {
	var files []pluginFile
	var total int64
	err := fs.WalkDir(skill.root, skill.dir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link %q is not allowed", name)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxSkillFileBytes {
			return fmt.Errorf("%q is larger than %d bytes", name, maxSkillFileBytes)
		}
		if total += info.Size(); total > maxSkillBytes {
			return fmt.Errorf("skill is larger than %d bytes", maxSkillBytes)
		}
		data, err := fs.ReadFile(skill.root, name)
		if err != nil {
			return err
		}
		rel := path.Join("skills", skill.Name, strings.TrimPrefix(name, skill.dir+"/"))
		files = append(files, pluginFile{rel: rel, data: data})
		return nil
	})
	return files, err
}

// Materialize writes the skills into an immutable plugin folder named by the
// content hash under root and returns it. An existing folder is reused, so
// running sessions never see their plugin change. A skill that cannot be read
// is skipped and reported in the returned warnings.
func Materialize(root string, chosen []Skill) (dir string, warnings []string, err error) {
	var files []pluginFile
	for _, skill := range chosen {
		skillFiles, err := collectFiles(skill)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipped skill %q: %v", skill.Name, err))
			continue
		}
		files = append(files, skillFiles...)
	}
	if len(files) == 0 {
		return "", warnings, nil
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	hash := sha256.New()
	for _, f := range files {
		fmt.Fprintf(hash, "%s\x00%d\x00", f.rel, len(f.data))
		hash.Write(f.data)
	}
	target := filepath.Join(root, hex.EncodeToString(hash.Sum(nil))[:12])
	if _, statErr := os.Stat(target); statErr == nil {
		now := time.Now()
		_ = os.Chtimes(target, now, now)
		return target, warnings, nil
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", warnings, err
	}
	temp, err := os.MkdirTemp(root, ".tmp-")
	if err != nil {
		return "", warnings, err
	}
	write := func(rel string, data []byte) error {
		full := filepath.Join(temp, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			return err
		}
		return os.WriteFile(full, data, 0o600)
	}
	manifest := []byte(`{"name":"` + PluginName + `","description":"Skills provided by TauShell","version":"1.0.0"}` + "\n")
	failure := write(".claude-plugin/plugin.json", manifest)
	if failure == nil {
		failure = write("plugin.json", manifest)
	}
	for _, f := range files {
		if failure != nil {
			break
		}
		failure = write(f.rel, f.data)
	}
	if failure == nil {
		failure = os.Rename(temp, target)
		if failure != nil {
			if _, statErr := os.Stat(target); statErr == nil {
				failure = nil
			}
		}
	}
	if failure != nil {
		return "", warnings, errors.Join(failure, os.RemoveAll(temp))
	}
	_ = os.RemoveAll(temp)
	return target, warnings, nil
}

// GC removes plugin folders untouched for longer than maxAge.
func GC(root string, maxAge time.Duration) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err == nil && entry.IsDir() && time.Since(info.ModTime()) > maxAge {
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
}
