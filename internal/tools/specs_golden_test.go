package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// The harness specs are generated from the built-in manifests; this golden
// file proves they match what was hand-written before manifests existed.
func TestHarnessSpecsMatchGolden(t *testing.T) {
	var harnesses []Spec
	for _, spec := range Specs() {
		switch spec.ID {
		case PowerShell, NodeJS, Git:
		default:
			harnesses = append(harnesses, spec)
		}
	}
	sort.Slice(harnesses, func(i, j int) bool { return harnesses[i].ID < harnesses[j].ID })
	got, err := json.MarshalIndent(harnesses, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "harness_specs.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Fatalf("harness specs differ from %s; rerun with UPDATE_GOLDEN=1 only if the change is intended\n%s", path, got)
	}
}
