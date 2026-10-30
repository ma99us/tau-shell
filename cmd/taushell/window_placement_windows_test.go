//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

func TestWindowPlacementRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "window-placement.json")
	if _, err := loadWindowPlacement(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing placement: %v", err)
	}
	want := windowPlacement{ShowCmd: 1, NormalBounds: placementRect{-1400, 100, -120, 1020}}
	want.Length = uint32(unsafe.Sizeof(want))
	for _, state := range []uint32{1, 3} {
		want.ShowCmd = state
		if err := writeWindowPlacement(path, want); err != nil {
			t.Fatal(err)
		}
		got, err := loadWindowPlacement(path)
		if err != nil || got != want {
			t.Fatalf("round trip = %+v, %v; want %+v", got, err, want)
		}
	}
}

func TestWindowPlacementNormalizesMinimizedState(t *testing.T) {
	for _, test := range []struct {
		name  string
		show  uint32
		flags uint32
		want  uint32
	}{
		{"normal", 1, 0, 1},
		{"maximized", 3, 0, 3},
		{"minimized from normal", 2, 0, 1},
		{"minimized from maximized", 2, 2, 3},
		{"unknown state", 99, 0, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			placement, err := (windowPlacement{
				ShowCmd: test.show, Flags: test.flags,
				NormalBounds: placementRect{100, 100, 1380, 1020},
			}).normalized()
			if err != nil || placement.ShowCmd != test.want || placement.Flags != 0 || placement.Length != 44 {
				t.Fatalf("normalized = %+v, %v", placement, err)
			}
		})
	}
}

func TestWindowPlacementRejectsInvalidState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window-placement.json")
	for _, data := range []string{`{`, `{}`, `{"NormalBounds":{"Left":100,"Top":100,"Right":200,"Bottom":200}}`} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadWindowPlacement(path); err == nil {
			t.Fatalf("accepted invalid state %q", data)
		}
	}
}
