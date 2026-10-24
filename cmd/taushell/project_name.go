package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var tomlNamePattern = regexp.MustCompile(`^name\s*=\s*["']([^"']+)["']`)

// deduceSessionName returns the project's own name when the directory holds a
// recognizable project manifest, otherwise the directory's base name.
func deduceSessionName(directory string) string {
	if name := projectManifestName(directory); name != "" {
		return name
	}
	cleaned := filepath.Clean(directory)
	base := filepath.Base(cleaned)
	if base == "." || base == string(filepath.Separator) || strings.HasSuffix(base, ":") || strings.HasSuffix(base, `:\`) {
		return strings.TrimRight(filepath.VolumeName(cleaned), ":")
	}
	return base
}

func projectManifestName(directory string) string {
	if data, err := os.ReadFile(filepath.Join(directory, "package.json")); err == nil {
		var manifest struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(data, &manifest) == nil {
			if name := strings.TrimSpace(manifest.Name[strings.LastIndex(manifest.Name, "/")+1:]); name != "" {
				return name
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(directory, "go.mod")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if module, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
				module = strings.Trim(strings.TrimSpace(module), `"`)
				if name := module[strings.LastIndex(module, "/")+1:]; name != "" {
					return name
				}
				break
			}
		}
	}
	for _, manifest := range []struct{ file, section string }{{"Cargo.toml", "[package]"}, {"pyproject.toml", "[project]"}} {
		if name := tomlSectionName(filepath.Join(directory, manifest.file), manifest.section); name != "" {
			return name
		}
	}
	return ""
}

func tomlSectionName(path, section string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	inSection := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") {
			inSection = line == section
			continue
		}
		if inSection {
			if match := tomlNamePattern.FindStringSubmatch(line); match != nil {
				return match[1]
			}
		}
	}
	return ""
}
