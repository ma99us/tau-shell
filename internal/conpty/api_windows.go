package conpty

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// pseudoConsoleAPI abstracts the inbox kernel32 ConPTY from a bundled
// conpty.dll (Microsoft.Windows.Console.ConPTY), which hosts sessions through
// the OpenConsole.exe located next to the DLL.
type pseudoConsoleAPI struct {
	name   string
	create func(size windows.Coord, input, output windows.Handle, flags uint32, console *windows.Handle) error
	resize func(console windows.Handle, size windows.Coord) error
	close  func(console windows.Handle)
}

var inboxAPI = pseudoConsoleAPI{
	name:   BackendInbox,
	create: windows.CreatePseudoConsole,
	resize: windows.ResizePseudoConsole,
	close:  windows.ClosePseudoConsole,
}

var (
	bundledMu   sync.Mutex
	bundledAPIs = map[string]pseudoConsoleAPI{}
)

func loadBundledAPI(library string) (pseudoConsoleAPI, error) {
	if !filepath.IsAbs(library) {
		return pseudoConsoleAPI{}, fmt.Errorf("bundled ConPTY library path must be absolute: %s", library)
	}
	bundledMu.Lock()
	defer bundledMu.Unlock()
	if api, ok := bundledAPIs[library]; ok {
		return api, nil
	}
	dll, err := windows.LoadDLL(library)
	if err != nil {
		return pseudoConsoleAPI{}, fmt.Errorf("load bundled ConPTY %s: %w", library, err)
	}
	procs := map[string]*windows.Proc{}
	for _, name := range []string{"ConptyCreatePseudoConsole", "ConptyResizePseudoConsole", "ConptyClosePseudoConsole"} {
		proc, err := dll.FindProc(name)
		if err != nil {
			_ = dll.Release()
			return pseudoConsoleAPI{}, fmt.Errorf("locate %s in %s: %w", name, library, err)
		}
		procs[name] = proc
	}
	api := pseudoConsoleAPI{
		name: BackendBundled,
		create: func(size windows.Coord, input, output windows.Handle, flags uint32, console *windows.Handle) error {
			result, _, _ := procs["ConptyCreatePseudoConsole"].Call(
				coordArg(size), uintptr(input), uintptr(output), uintptr(flags), uintptr(unsafe.Pointer(console)))
			return hresultError("ConptyCreatePseudoConsole", result)
		},
		resize: func(console windows.Handle, size windows.Coord) error {
			result, _, _ := procs["ConptyResizePseudoConsole"].Call(uintptr(console), coordArg(size))
			return hresultError("ConptyResizePseudoConsole", result)
		},
		close: func(console windows.Handle) {
			_, _, _ = procs["ConptyClosePseudoConsole"].Call(uintptr(console))
		},
	}
	bundledAPIs[library] = api
	return api, nil
}

const (
	BackendInbox   = "inbox"
	BackendBundled = "bundled"

	bundledLibraryName = "conpty.dll"
	bundledHostName    = "OpenConsole.exe"
)

// BundledLibrary returns the absolute conpty.dll path when both conpty.dll and
// OpenConsole.exe are present in directory, or "" otherwise.
func BundledLibrary(directory string) string {
	if directory == "" {
		return ""
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return ""
	}
	for _, name := range []string{bundledLibraryName, bundledHostName} {
		info, err := os.Stat(filepath.Join(absolute, name))
		if err != nil || info.IsDir() {
			return ""
		}
	}
	return filepath.Join(absolute, bundledLibraryName)
}

// ExecutableDirectory returns the directory containing the running executable,
// where packaged builds place the bundled ConPTY files.
func ExecutableDirectory() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(executable)
}

// StartPreferred uses the bundled ConPTY found in libraryDirectory and falls
// back to inbox ConPTY when it is absent or cannot be loaded. fallbackErr
// reports why a present bundle was not used so callers can surface it.
func StartPreferred(libraryDirectory string, width, height int, executable, directory string, args ...string) (process *Process, fallbackErr error, err error) {
	return StartPreferredEnv(libraryDirectory, nil, width, height, executable, directory, args...)
}

// StartPreferredEnv is StartPreferred with an explicit child environment
// block (nil inherits the current environment).
func StartPreferredEnv(libraryDirectory string, env []string, width, height int, executable, directory string, args ...string) (process *Process, fallbackErr error, err error) {
	if library := BundledLibrary(libraryDirectory); library != "" {
		api, loadErr := loadBundledAPI(library)
		if loadErr == nil {
			process, err = start(api, width, height, executable, directory, env, args...)
			return process, nil, err
		}
		fallbackErr = loadErr
	}
	process, err = start(inboxAPI, width, height, executable, directory, env, args...)
	return process, fallbackErr, err
}

func coordArg(size windows.Coord) uintptr {
	return uintptr(*(*uint32)(unsafe.Pointer(&size)))
}

func hresultError(operation string, result uintptr) error {
	if int32(result) < 0 {
		return fmt.Errorf("%s failed: HRESULT 0x%08x", operation, uint32(result))
	}
	return nil
}
