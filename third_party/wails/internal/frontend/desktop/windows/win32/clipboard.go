//go:build windows

/*
 * Based on code originally from https://github.com/atotto/clipboard. Copyright (c) 2013 Ato Araki. All rights reserved.
 */

package win32

import (
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const errClipboardNotOpen = syscall.Errno(1418)

const (
	cfUnicodetext = 13
	gmemMoveable  = 0x0002
)

// waitOpenClipboard opens the clipboard, waiting for up to a second to do so.
func waitOpenClipboard() error {
	started := time.Now()
	limit := started.Add(time.Second)
	var r uintptr
	var err error
	for time.Now().Before(limit) {
		r, _, err = procOpenClipboard.Call(0)
		if r != 0 {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	return err
}

// retryClipboard repeats an operation a few times when Windows reports that the clipboard is not open.
// Another process (clipboard manager, remote session) can take it between our calls.
func retryClipboard(operation func() error) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if err = operation(); err != errClipboardNotOpen {
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return err
}

func GetClipboardText() (text string, err error) {
	err = retryClipboard(func() error {
		var inner error
		text, inner = getClipboardText()
		return inner
	})
	return text, err
}

func SetClipboardText(text string) error {
	return retryClipboard(func() error { return setClipboardText(text) })
}

func getClipboardText() (string, error) {
	// LockOSThread ensure that the whole method will keep executing on the same thread from begin to end (it actually locks the goroutine thread attribution).
	// Otherwise if the goroutine switch thread during execution (which is a common practice), the OpenClipboard and CloseClipboard will happen on two different threads, and it will result in a clipboard deadlock.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// No text (for example an image-only clipboard) is an empty result. The last error is stale here, so it is never reported.
	if formatAvailable, _, _ := procIsClipboardFormatAvailable.Call(cfUnicodetext); formatAvailable == 0 {
		return "", nil
	}
	err := waitOpenClipboard()
	if err != nil {
		return "", err
	}

	h, _, err := procGetClipboardData.Call(cfUnicodetext)
	if h == 0 {
		_, _, _ = procCloseClipboard.Call()
		return "", err
	}

	l, _, err := kernelGlobalLock.Call(h)
	if l == 0 {
		_, _, _ = procCloseClipboard.Call()
		return "", err
	}

	text := syscall.UTF16ToString((*[1 << 20]uint16)(unsafe.Pointer(l))[:])

	r, _, err := kernelGlobalUnlock.Call(h)
	if r == 0 {
		_, _, _ = procCloseClipboard.Call()
		return "", err
	}

	closed, _, err := procCloseClipboard.Call()
	if closed == 0 {
		return "", err
	}
	return text, nil
}

func setClipboardText(text string) error {
	// LockOSThread ensure that the whole method will keep executing on the same thread from begin to end (it actually locks the goroutine thread attribution).
	// Otherwise if the goroutine switch thread during execution (which is a common practice), the OpenClipboard and CloseClipboard will happen on two different threads, and it will result in a clipboard deadlock.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	err := waitOpenClipboard()
	if err != nil {
		return err
	}

	r, _, err := procEmptyClipboard.Call(0)
	if r == 0 {
		_, _, _ = procCloseClipboard.Call()
		return err
	}

	data, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}

	// "If the hMem parameter identifies a memory object, the object must have
	// been allocated using the function with the GMEM_MOVEABLE flag."
	h, _, err := kernelGlobalAlloc.Call(gmemMoveable, uintptr(len(data)*int(unsafe.Sizeof(data[0]))))
	if h == 0 {
		_, _, _ = procCloseClipboard.Call()
		return err
	}
	defer func() {
		if h != 0 {
			kernelGlobalFree.Call(h)
		}
	}()

	l, _, err := kernelGlobalLock.Call(h)
	if l == 0 {
		_, _, _ = procCloseClipboard.Call()
		return err
	}

	r, _, err = kernelLstrcpy.Call(l, uintptr(unsafe.Pointer(&data[0])))
	if r == 0 {
		_, _, _ = procCloseClipboard.Call()
		return err
	}

	r, _, err = kernelGlobalUnlock.Call(h)
	if r == 0 {
		// The clipboard is open here, so a "not open" code is a stale last error.
		if errno, _ := err.(syscall.Errno); errno != 0 && errno != errClipboardNotOpen {
			_, _, _ = procCloseClipboard.Call()
			return err
		}
	}

	r, _, err = procSetClipboardData.Call(cfUnicodetext, h)
	if r == 0 {
		_, _, _ = procCloseClipboard.Call()
		return err
	}
	h = 0 // suppress deferred cleanup
	closed, _, err := procCloseClipboard.Call()
	if closed == 0 {
		return err
	}
	return nil
}
