//go:build windows

package pairing

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DPAPISealer protects data with the current Windows user's key
// (CryptProtectData). Another Windows account, or a copy of the file on another
// machine, cannot open it. Malware running as the same user still can, which is
// the same trust level as the rest of ~/.tau_shell.
type DPAPISealer struct{}

var entropy = []byte("taushell-mobile-v1")

func blob(data []byte) *windows.DataBlob {
	if len(data) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
}

func (DPAPISealer) Seal(plain []byte) ([]byte, error) {
	var out windows.DataBlob
	if err := windows.CryptProtectData(blob(plain), nil, blob(entropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func (DPAPISealer) Open(sealed []byte) ([]byte, error) {
	if len(sealed) == 0 {
		return nil, errors.New("nothing to unseal")
	}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blob(sealed), nil, blob(entropy), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
