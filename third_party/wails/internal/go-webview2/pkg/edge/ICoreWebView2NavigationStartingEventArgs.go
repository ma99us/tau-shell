//go:build windows

package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type ICoreWebView2NavigationStartingEventArgsVtbl struct {
	_IUnknownVtbl
	GetURI             ComProc
	GetIsUserInitiated ComProc
	GetIsRedirected    ComProc
	GetRequestHeaders  ComProc
	GetCancel          ComProc
	PutCancel          ComProc
	GetNavigationID    ComProc
}

type ICoreWebView2NavigationStartingEventArgs struct {
	vtbl *ICoreWebView2NavigationStartingEventArgsVtbl
}

func (i *ICoreWebView2NavigationStartingEventArgs) GetURI() (string, error) {
	var uri *uint16
	hr, _, _ := i.vtbl.GetURI.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&uri)),
	)
	if windows.Handle(hr) != windows.S_OK {
		return "", windows.Errno(hr)
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(uri))
	return windows.UTF16PtrToString(uri), nil
}

func (i *ICoreWebView2NavigationStartingEventArgs) PutCancel(cancel bool) error {
	var value uintptr
	if cancel {
		value = 1
	}
	hr, _, _ := i.vtbl.PutCancel.Call(uintptr(unsafe.Pointer(i)), value)
	if windows.Handle(hr) != windows.S_OK {
		return windows.Errno(hr)
	}
	return nil
}
