package vault

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"syscall"
	"unsafe"
)

const (
	credTypeGeneric      = 1
	credPersistLocal     = 2 // survives logoff on this machine, never roams
	errorNotFound        = syscall.Errno(1168)
	targetPrefix         = "TauShell/secret/"
	credentialEnumFilter = targetPrefix + "*"
)

var (
	advapi         = syscall.NewLazyDLL("advapi32.dll")
	procCredWrite  = advapi.NewProc("CredWriteW")
	procCredRead   = advapi.NewProc("CredReadW")
	procCredDelete = advapi.NewProc("CredDeleteW")
	procCredEnum   = advapi.NewProc("CredEnumerateW")
	procCredFree   = advapi.NewProc("CredFree")
)

type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// CredentialManager stores secrets as generic credentials of the current
// Windows user, which Windows protects with that user's login.
type CredentialManager struct{}

func NewCredentialManager() *CredentialManager { return &CredentialManager{} }

func target(name string) (*uint16, error) {
	return syscall.UTF16PtrFromString(targetPrefix + name)
}

func (CredentialManager) Set(name, value string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if err := validateValue(value); err != nil {
		return err
	}
	targetName, err := target(name)
	if err != nil {
		return err
	}
	blob := []byte(value)
	entry := credential{
		Type:               credTypeGeneric,
		TargetName:         targetName,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocal,
	}
	result, _, callErr := procCredWrite.Call(uintptr(unsafe.Pointer(&entry)), 0)
	for index := range blob {
		blob[index] = 0
	}
	if result == 0 {
		return fmt.Errorf("store secret %q: %w", name, callErr)
	}
	return nil
}

func (CredentialManager) Get(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	targetName, err := target(name)
	if err != nil {
		return "", err
	}
	var entry *credential
	result, _, callErr := procCredRead.Call(uintptr(unsafe.Pointer(targetName)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&entry)))
	if result == 0 {
		if errors.Is(callErr, errorNotFound) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("read secret %q: %w", name, callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(entry)))
	if entry.CredentialBlobSize == 0 || entry.CredentialBlob == nil {
		return "", nil
	}
	return string(unsafe.Slice(entry.CredentialBlob, entry.CredentialBlobSize)), nil
}

func (CredentialManager) Delete(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	targetName, err := target(name)
	if err != nil {
		return err
	}
	result, _, callErr := procCredDelete.Call(uintptr(unsafe.Pointer(targetName)), credTypeGeneric, 0)
	if result == 0 {
		if errors.Is(callErr, errorNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("delete secret %q: %w", name, callErr)
	}
	return nil
}

func (CredentialManager) Names() ([]string, error) {
	filter, err := syscall.UTF16PtrFromString(credentialEnumFilter)
	if err != nil {
		return nil, err
	}
	var count uint32
	var entries **credential
	result, _, callErr := procCredEnum.Call(uintptr(unsafe.Pointer(filter)), 0, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&entries)))
	if result == 0 {
		if errors.Is(callErr, errorNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("list secrets: %w", callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(entries)))
	names := make([]string, 0, count)
	for _, entry := range unsafe.Slice(entries, count) {
		if name, ok := strings.CutPrefix(utf16String(entry.TargetName), targetPrefix); ok && ValidateName(name) == nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// utf16String reads a NUL-terminated UTF-16 string, stopping after a bound.
func utf16String(pointer *uint16) string {
	if pointer == nil {
		return ""
	}
	var units []uint16
	for offset := uintptr(0); offset < 1024; offset += 2 {
		unit := *(*uint16)(unsafe.Add(unsafe.Pointer(pointer), offset))
		if unit == 0 {
			break
		}
		units = append(units, unit)
	}
	return syscall.UTF16ToString(append(units, 0))
}
