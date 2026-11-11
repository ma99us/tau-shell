//go:build windows

package ide

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func init() { uninstallEntries = readUninstallEntries }

func readUninstallEntries() []UninstallEntry {
	const base = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`
	var entries []UninstallEntry
	for _, root := range []struct {
		key    registry.Key
		access uint32
		path   string
	}{
		{registry.LOCAL_MACHINE, registry.READ | registry.WOW64_64KEY, base},
		{registry.LOCAL_MACHINE, registry.READ | registry.WOW64_32KEY, base},
		{registry.CURRENT_USER, registry.READ, base},
	} {
		parent, err := registry.OpenKey(root.key, root.path, registry.ENUMERATE_SUB_KEYS|root.access)
		if err != nil {
			continue
		}
		names, _ := parent.ReadSubKeyNames(-1)
		parent.Close()
		for _, name := range names {
			child, err := registry.OpenKey(root.key, root.path+`\`+name, registry.QUERY_VALUE|root.access)
			if err != nil {
				continue
			}
			display, _, _ := child.GetStringValue("DisplayName")
			location, _, _ := child.GetStringValue("InstallLocation")
			child.Close()
			if display != "" {
				entries = append(entries, UninstallEntry{DisplayName: display, InstallLocation: location})
			}
		}
	}
	return entries
}

var (
	shell32           = windows.NewLazySystemDLL("shell32.dll")
	user32            = windows.NewLazySystemDLL("user32.dll")
	gdi32             = windows.NewLazySystemDLL("gdi32.dll")
	procSHGetFileInfo = shell32.NewProc("SHGetFileInfoW")
	procGetIconInfo   = user32.NewProc("GetIconInfo")
	procDestroyIcon   = user32.NewProc("DestroyIcon")
	procGetDC         = user32.NewProc("GetDC")
	procReleaseDC     = user32.NewProc("ReleaseDC")
	procGetObject     = gdi32.NewProc("GetObjectW")
	procGetDIBits     = gdi32.NewProc("GetDIBits")
	procDeleteObject  = gdi32.NewProc("DeleteObject")
)

const (
	shgfiIcon      = 0x100
	shgfiLargeIcon = 0x0
)

type shFileInfo struct {
	Icon        syscall.Handle
	IconIndex   int32
	Attributes  uint32
	DisplayName [260]uint16
	TypeName    [80]uint16
}

type iconInfo struct {
	IsIcon   int32
	XHotspot uint32
	YHotspot uint32
	Mask     syscall.Handle
	Color    syscall.Handle
}

type bitmap struct {
	Type       int32
	Width      int32
	Height     int32
	WidthBytes int32
	Planes     uint16
	BitsPixel  uint16
	Bits       uintptr
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

var (
	iconCache   = map[string]string{}
	iconCacheMu sync.Mutex
)

// IconDataURL returns the executable's own icon as a PNG data URL, or "" when
// it cannot be read. Results are cached per executable.
func IconDataURL(exe string) string {
	iconCacheMu.Lock()
	defer iconCacheMu.Unlock()
	if cached, ok := iconCache[exe]; ok {
		return cached
	}
	encoded := ""
	if img := extractIcon(exe); img != nil {
		var buffer bytes.Buffer
		if png.Encode(&buffer, img) == nil {
			encoded = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
		}
	}
	iconCache[exe] = encoded
	return encoded
}

func extractIcon(exe string) image.Image {
	path, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return nil
	}
	var info shFileInfo
	result, _, _ := procSHGetFileInfo.Call(uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info), shgfiIcon|shgfiLargeIcon)
	if result == 0 || info.Icon == 0 {
		return nil
	}
	defer procDestroyIcon.Call(uintptr(info.Icon))
	var icon iconInfo
	if ok, _, _ := procGetIconInfo.Call(uintptr(info.Icon), uintptr(unsafe.Pointer(&icon))); ok == 0 {
		return nil
	}
	defer procDeleteObject.Call(uintptr(icon.Mask))
	if icon.Color == 0 {
		return nil
	}
	defer procDeleteObject.Call(uintptr(icon.Color))
	var bmp bitmap
	if n, _, _ := procGetObject.Call(uintptr(icon.Color), unsafe.Sizeof(bmp), uintptr(unsafe.Pointer(&bmp))); n == 0 {
		return nil
	}
	width, height := int(bmp.Width), int(bmp.Height)
	if width <= 0 || height <= 0 || width > 512 || height > 512 {
		return nil
	}
	header := bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(width), Height: -int32(height), Planes: 1, BitCount: 32}
	pixels := make([]byte, width*height*4)
	dc, _, _ := procGetDC.Call(0)
	if dc == 0 {
		return nil
	}
	defer procReleaseDC.Call(0, dc)
	if n, _, _ := procGetDIBits.Call(dc, uintptr(icon.Color), 0, uintptr(height), uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&header)), 0); n == 0 {
		return nil
	}
	opaque := true
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] != 0 {
			opaque = false
			break
		}
	}
	// Icons without an alpha channel report all zeros; treat them as opaque.
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < len(pixels); i += 4 {
		alpha := pixels[i+3]
		if opaque {
			alpha = 255
		}
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = pixels[i+2], pixels[i+1], pixels[i], alpha
	}
	return img
}
