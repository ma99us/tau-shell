//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/ma99us/tau-shell/internal/diagnostics"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	procGetWindowPlacement = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement = user32.NewProc("SetWindowPlacement")
)

type placementPoint struct {
	X int32
	Y int32
}

type placementRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type windowPlacement struct {
	Length       uint32
	Flags        uint32
	ShowCmd      uint32
	MinPosition  placementPoint
	MaxPosition  placementPoint
	NormalBounds placementRect
}

func (placement windowPlacement) normalized() (windowPlacement, error) {
	bounds := placement.NormalBounds
	if int64(bounds.Right)-int64(bounds.Left) < 760 || int64(bounds.Bottom)-int64(bounds.Top) < 520 {
		return windowPlacement{}, errors.New("saved window bounds are too small or invalid")
	}
	if placement.ShowCmd == 3 || (placement.ShowCmd == 2 && placement.Flags&2 != 0) {
		placement.ShowCmd = 3
	} else {
		placement.ShowCmd = 1
	}
	placement.Flags = 0
	placement.Length = uint32(unsafe.Sizeof(placement))
	return placement, nil
}

func loadWindowPlacement(path string) (windowPlacement, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return windowPlacement{}, err
	}
	var placement windowPlacement
	if err := json.Unmarshal(data, &placement); err != nil {
		return windowPlacement{}, err
	}
	return placement.normalized()
}

func writeWindowPlacement(path string, placement windowPlacement) error {
	placement, err := placement.normalized()
	if err != nil {
		return err
	}
	data, err := json.Marshal(placement)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".window-placement-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	_, writeErr := temporary.Write(data)
	closeErr := temporary.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

func (application *App) windowPlacementPath() string {
	return filepath.Join(filepath.Dir(application.configDirectory), "window-placement.json")
}

func (application *App) restoreWindowPlacement(ctx context.Context) {
	runtime.WindowShow(ctx)
	placement, err := loadWindowPlacement(application.windowPlacementPath())
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		diagnostics.Warn("load window placement failed; using default window", err)
		return
	}
	hwnd := findMainWindowHandle(false)
	if hwnd == 0 {
		diagnostics.Warn("restore window placement failed", errors.New("main window not found"))
		return
	}
	if result, _, callErr := procSetWindowPlacement.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&placement))); result == 0 {
		diagnostics.Warn("restore window placement failed", callErr)
	}
}

func (application *App) saveWindowPlacement() {
	if application.configDirectory == "" {
		return
	}
	hwnd := mainWindowHandle()
	if hwnd == 0 {
		diagnostics.Warn("save window placement failed", errors.New("main window not found"))
		return
	}
	placement := windowPlacement{Length: uint32(unsafe.Sizeof(windowPlacement{}))}
	if result, _, err := procGetWindowPlacement.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&placement))); result == 0 {
		diagnostics.Warn("read window placement failed", err)
		return
	}
	if err := writeWindowPlacement(application.windowPlacementPath(), placement); err != nil {
		diagnostics.Warn("save window placement failed", fmt.Errorf("write window state: %w", err))
	}
}
