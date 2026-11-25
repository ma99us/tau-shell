package main

import (
	"fmt"
	"unsafe"
)

const (
	vkLWin           = 0x5B
	vkH              = 0x48
	inputKeyboard    = 1
	keyEventKeyUp    = 0x0002
	keyboardInputLen = 40
)

var procSendInput = user32.NewProc("SendInput")

// keyInput mirrors the 64-bit Win32 INPUT structure for a keyboard event.
type keyInput struct {
	inputType uint32
	_         uint32
	vk        uint16
	scan      uint16
	flags     uint32
	time      uint32
	_         uint32
	extra     uintptr
	_         [8]byte
}

// ToggleVoiceTyping presses Win+H, which opens or closes the Windows Voice
// typing panel for the focused control. Windows draws that panel; TauShell
// cannot hide it or read whether it is listening.
func (application *App) ToggleVoiceTyping() error {
	if unsafe.Sizeof(keyInput{}) != keyboardInputLen {
		return fmt.Errorf("voice typing is not supported on this architecture")
	}
	events := []keyInput{
		{inputType: inputKeyboard, vk: vkLWin},
		{inputType: inputKeyboard, vk: vkH},
		{inputType: inputKeyboard, vk: vkH, flags: keyEventKeyUp},
		{inputType: inputKeyboard, vk: vkLWin, flags: keyEventKeyUp},
	}
	sent, _, err := procSendInput.Call(uintptr(len(events)), uintptr(unsafe.Pointer(&events[0])), unsafe.Sizeof(events[0]))
	if int(sent) != len(events) {
		return fmt.Errorf("Windows did not accept the Win+H key press: %v", err)
	}
	return nil
}
