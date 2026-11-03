// Package projectfiles lists and reads files inside one project folder, for the
// read-only file drawer. Every path is relative to the root and is resolved
// (symlinks included) before use, so nothing outside the root can be reached.
package projectfiles

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// MaxReadBytes bounds how much of a file the viewer loads.
const MaxReadBytes = 512 * 1024

// MaxEntries bounds one directory listing so a huge folder stays responsive.
const MaxEntries = 2000

// ErrOutside is returned for any path that resolves outside the root.
var ErrOutside = errors.New("that path is outside the project folder")

// Entry is one item of a directory listing. Path uses forward slashes and is relative to the root.
type Entry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
	Size int64  `json:"size"`
}

// Listing is a directory's entries, folders first, plus whether the list was cut short.
type Listing struct {
	Entries   []Entry `json:"entries"`
	Truncated bool    `json:"truncated"`
}

// File is the readable text of one file.
type File struct {
	Path      string `json:"path"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

var noise = map[string]bool{".git": true, "node_modules": true, ".vs": true, "__pycache__": true}

// Resolve returns the absolute, symlink-free path of rel inside root.
func Resolve(root, rel string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("project folder: %w", err)
	}
	realRoot, err = filepath.Abs(realRoot)
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.VolumeName(clean) != "" {
		return "", ErrOutside
	}
	target, err := filepath.EvalSymlinks(filepath.Join(realRoot, clean))
	if err != nil {
		return "", err
	}
	if !within(realRoot, target) {
		return "", ErrOutside
	}
	return target, nil
}

func within(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// List returns the entries of directory rel. Noise folders (.git, node_modules, ...) are
// hidden unless showAll is set. Entries that are links leaving the root are omitted.
func List(root, rel string, showAll bool) (Listing, error) {
	dir, err := Resolve(root, rel)
	if err != nil {
		return Listing{}, err
	}
	realRoot, _ := filepath.EvalSymlinks(root)
	items, err := os.ReadDir(dir)
	if err != nil {
		return Listing{}, err
	}
	listing := Listing{Entries: []Entry{}}
	base := strings.Trim(filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))), "./")
	for _, item := range items {
		if !showAll && noise[item.Name()] {
			continue
		}
		full := filepath.Join(dir, item.Name())
		info, err := os.Stat(full)
		if err != nil {
			continue
		}
		if item.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(full)
			if err != nil || !within(realRoot, resolved) {
				continue
			}
		}
		if len(listing.Entries) >= MaxEntries {
			listing.Truncated = true
			break
		}
		path := item.Name()
		if base != "" {
			path = base + "/" + item.Name()
		}
		listing.Entries = append(listing.Entries, Entry{Name: item.Name(), Path: path, Dir: info.IsDir(), Size: info.Size()})
	}
	sort.SliceStable(listing.Entries, func(i, j int) bool {
		a, b := listing.Entries[i], listing.Entries[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return listing, nil
}

// Read returns the text of file rel, at most MaxReadBytes. Binary files are refused.
func Read(root, rel string) (File, error) {
	path, err := Resolve(root, rel)
	if err != nil {
		return File{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return File{}, err
	}
	if info.IsDir() {
		return File{}, errors.New("that is a folder, not a file")
	}
	handle, err := os.Open(path)
	if err != nil {
		return File{}, err
	}
	defer handle.Close()
	data, err := io.ReadAll(io.LimitReader(handle, MaxReadBytes+1))
	if err != nil {
		return File{}, err
	}
	truncated := len(data) > MaxReadBytes
	if truncated {
		data = data[:MaxReadBytes]
		// Do not leave half a multi-byte character at the cut.
		for i := 0; i < utf8.UTFMax && len(data) > 0 && !utf8.Valid(data); i++ {
			data = data[:len(data)-1]
		}
	}
	if strings.ContainsRune(string(data), 0) || !utf8.Valid(data) {
		return File{}, errors.New("this file is not UTF-8 text, so it cannot be shown")
	}
	return File{Path: strings.TrimPrefix(filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))), "./"), Text: string(data), Truncated: truncated}, nil
}
