//go:build windows
// +build windows

package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	shell32                = syscall.NewLazyDLL("shell32.dll")
	procShellNotifyIcon    = shell32.NewProc("Shell_NotifyIconW")
	procExtractIcon        = shell32.NewProc("ExtractIconW")

	procRegisterClassEx    = user32.NewProc("RegisterClassExW")
	procUnregisterClass    = user32.NewProc("UnregisterClassW")
	procCreateWindowEx     = user32.NewProc("CreateWindowExW")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procDefWindowProc      = user32.NewProc("DefWindowProcW")
	procGetMessage         = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessage    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procCreatePopupMenu    = user32.NewProc("CreatePopupMenu")
	procDestroyMenu        = user32.NewProc("DestroyMenu")
	procAppendMenu         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu     = user32.NewProc("TrackPopupMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetCursorPos       = user32.NewProc("GetCursorPos")
	procLoadIcon           = user32.NewProc("LoadIconW")
	procLoadImage          = user32.NewProc("LoadImageW")

	procGetModuleHandle    = kernel32.NewProc("GetModuleHandleW")
	procGetModuleFileName  = kernel32.NewProc("GetModuleFileNameW")
)

const (
	NIM_ADD        = 0x00000000
	NIM_MODIFY     = 0x00000001
	NIM_DELETE     = 0x00000002
	NIM_SETVERSION = 0x00000004

	NIF_MESSAGE = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_TIP     = 0x00000004
	NIF_STATE   = 0x00000008
	NIF_INFO    = 0x00000010
	NIF_GUID    = 0x00000020

	NIIF_NONE    = 0x00000000
	NIIF_INFO    = 0x00000001
	NIIF_WARNING = 0x00000002
	NIIF_ERROR   = 0x00000003

	WM_USER          = 0x0400
	WM_TRAYICON      = WM_USER + 101
	WM_LBUTTONUP     = 0x0202
	WM_LBUTTONDBLCLK = 0x0203
	WM_RBUTTONUP     = 0x0205
	WM_CONTEXTMENU   = 0x007B
	WM_NULL          = 0x0000

	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD   = 0x0100

	MF_STRING    = 0x00000000
	MF_SEPARATOR = 0x00000800

	IMAGE_ICON      = 1
	LR_LOADFROMFILE = 0x00000010
	LR_DEFAULTSIZE  = 0x00000040
	LR_SHARED       = 0x00008000

	IDI_APPLICATION = 32512
	HWND_MESSAGE    = ^uintptr(2) // -3

	SW_HIDE = 0
	SW_SHOW = 5
)

// NOTIFYICONDATAW represents Windows tray notification data (976 bytes on win64).
type NOTIFYICONDATAW struct {
	CbSize           uint32
	_                uint32 // padding
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	_                uint32 // padding
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type POINT struct {
	X int32
	Y int32
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

// TrayCallbacks holds event handlers triggered by tray icon interactions.
type TrayCallbacks struct {
	OnShow        func()
	OnStartAll    func()
	OnPauseAll    func()
	OnAddDownload func()
	OnSettings    func()
	OnRestart     func()
	OnExit        func()
}

// TrayManager manages the desktop system notification area icon.
type TrayManager struct {
	mu        sync.Mutex
	hwnd      uintptr
	hIcon     uintptr
	nid       NOTIFYICONDATAW
	callbacks TrayCallbacks
	ready     chan struct{}
	stopped   bool
}

var (
	globalTray *TrayManager
	trayMu     sync.RWMutex
)

// GetGlobalTray returns the active tray manager instance if initialized.
func GetGlobalTray() *TrayManager {
	trayMu.RLock()
	defer trayMu.RUnlock()
	return globalTray
}

// NewTrayManager initializes and registers the desktop system tray icon.
func NewTrayManager(callbacks TrayCallbacks) (*TrayManager, error) {
	tm := &TrayManager{
		callbacks: callbacks,
		ready:     make(chan struct{}),
	}

	trayMu.Lock()
	globalTray = tm
	trayMu.Unlock()

	go tm.runMessageLoop()
	<-tm.ready

	return tm, nil
}

func (tm *TrayManager) runMessageLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInstance, _, _ := procGetModuleHandle.Call(0)
	className := syscall.StringToUTF16Ptr("NeoDMTrayClass")

	wndProc := syscall.NewCallback(func(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
		switch msg {
		case WM_TRAYICON:
			switch lParam {
			case WM_LBUTTONDBLCLK:
				if tm.callbacks.OnShow != nil {
					tm.callbacks.OnShow()
				}
				return 0

			case WM_LBUTTONUP:
				hwndApp := GetAppHWND()
				fg, _, _ := procGetForegroundWindow.Call()
				if hwndApp != 0 && fg == hwndApp {
					tm.showContextMenu(hwnd)
				} else if tm.callbacks.OnShow != nil {
					tm.callbacks.OnShow()
				}
				return 0

			case WM_RBUTTONUP, WM_CONTEXTMENU:
				tm.showContextMenu(hwnd)
				return 0
			}

		case WM_CLOSE:
			procDestroyWindow.Call(hwnd)
			return 0

		case 0x0002: // WM_DESTROY
			procPostQuitMessage.Call(0)
			return 0
		}

		r, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
		return r
	})

	wndClass := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   wndProc,
		HInstance:     hInstance,
		LpszClassName: className,
	}

	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wndClass)))

	// Create a top-level unowned hidden window.
	// NOTE: Shell_NotifyIcon MUST have an unowned top-level window (parent = 0), NOT HWND_MESSAGE.
	// Windows explicitly drops Shell_NotifyIcon callback messages sent to HWND_MESSAGE windows.
	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		0x80000000, // WS_POPUP (hidden, unowned top-level)
		0, 0, 0, 0,
		0, // Parent = 0 allows Shell_NotifyIcon callback delivery
		0,
		hInstance,
		0,
	)

	tm.mu.Lock()
	tm.hwnd = hwnd

	// Load tray icon:
	// 1. Try 16x16 icon from embedded Win32 resource ID 1
	hIcon, _, _ := procLoadImage.Call(hInstance, uintptr(1), uintptr(IMAGE_ICON), 16, 16, uintptr(LR_SHARED))

	// 2. Try loading from app.ico on disk if running local / dev
	if hIcon == 0 {
		currentExe := make([]uint16, 1024)
		procGetModuleFileName.Call(0, uintptr(unsafe.Pointer(&currentExe[0])), uintptr(len(currentExe)))
		exeDir := filepath.Dir(syscall.UTF16ToString(currentExe))
		candidates := []string{
			"app.ico",
			filepath.Join("frontend", "favicon.ico"),
			filepath.Join(exeDir, "app.ico"),
			filepath.Join(exeDir, "frontend", "favicon.ico"),
		}
		for _, cand := range candidates {
			if fi, err := os.Stat(cand); err == nil && fi.Size() > 0 {
				pUtf16 := syscall.StringToUTF16Ptr(cand)
				hIcon, _, _ = procLoadImage.Call(0, uintptr(unsafe.Pointer(pUtf16)), uintptr(IMAGE_ICON), 16, 16, uintptr(LR_LOADFROMFILE))
				if hIcon != 0 {
					break
				}
			}
		}
	}

	// 3. Fallback to ExtractIconW from running executable
	if hIcon == 0 || hIcon == 1 {
		exePathBuf := make([]uint16, 1024)
		procGetModuleFileName.Call(0, uintptr(unsafe.Pointer(&exePathBuf[0])), uintptr(len(exePathBuf)))
		hIcon, _, _ = procExtractIcon.Call(hInstance, uintptr(unsafe.Pointer(&exePathBuf[0])), 0)
	}

	// 4. Fallback to LoadIconW resource ID 1
	if hIcon == 0 || hIcon == 1 {
		hIcon, _, _ = procLoadIcon.Call(hInstance, uintptr(1))
	}

	// 5. Ultimate fallback to IDI_APPLICATION
	if hIcon == 0 || hIcon == 1 {
		hIcon, _, _ = procLoadIcon.Call(0, uintptr(IDI_APPLICATION))
	}
	tm.hIcon = hIcon

	// Prepare NOTIFYICONDATAW
	tm.nid = NOTIFYICONDATAW{
		CbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATAW{})),
		HWnd:             hwnd,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		UCallbackMessage: WM_TRAYICON,
		HIcon:            hIcon,
	}
	copyTip(&tm.nid.SzTip, "NeoDM PRO")

	procShellNotifyIcon.Call(NIM_ADD, uintptr(unsafe.Pointer(&tm.nid)))
	tm.mu.Unlock()

	close(tm.ready)

	// Windows Message Loop
	var msg MSG
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}

	tm.mu.Lock()
	procShellNotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(&tm.nid)))
	tm.stopped = true
	tm.mu.Unlock()
}

func (tm *TrayManager) showContextMenu(hwnd uintptr) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}

	appendMenuItem(hMenu, 1, "Open NeoDM")
	procAppendMenu.Call(hMenu, MF_SEPARATOR, 0, 0)
	appendMenuItem(hMenu, 2, "Start All Downloads")
	appendMenuItem(hMenu, 3, "Pause All Downloads")
	appendMenuItem(hMenu, 4, "Add Download...")
	procAppendMenu.Call(hMenu, MF_SEPARATOR, 0, 0)
	appendMenuItem(hMenu, 5, "Settings")
	procAppendMenu.Call(hMenu, MF_SEPARATOR, 0, 0)
	appendMenuItem(hMenu, 6, "Restart NeoDM")
	appendMenuItem(hMenu, 7, "Exit NeoDM")

	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	procSetForegroundWindow.Call(hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(hMenu, TPM_RIGHTBUTTON|TPM_RETURNCMD, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	procPostMessage.Call(hwnd, WM_NULL, 0, 0)
	procDestroyMenu.Call(hMenu)

	switch cmd {
	case 1:
		if tm.callbacks.OnShow != nil {
			tm.callbacks.OnShow()
		}
	case 2:
		if tm.callbacks.OnStartAll != nil {
			tm.callbacks.OnStartAll()
		}
	case 3:
		if tm.callbacks.OnPauseAll != nil {
			tm.callbacks.OnPauseAll()
		}
	case 4:
		if tm.callbacks.OnAddDownload != nil {
			tm.callbacks.OnAddDownload()
		}
	case 5:
		if tm.callbacks.OnSettings != nil {
			tm.callbacks.OnSettings()
		}
	case 6:
		if tm.callbacks.OnRestart != nil {
			tm.callbacks.OnRestart()
		} else {
			RestartApp()
		}
	case 7:
		if tm.callbacks.OnExit != nil {
			tm.callbacks.OnExit()
		} else {
			tm.Destroy()
			os.Exit(0)
		}
	}
}

func appendMenuItem(hMenu uintptr, id uintptr, text string) {
	utf16Ptr := syscall.StringToUTF16Ptr(text)
	procAppendMenu.Call(hMenu, MF_STRING, id, uintptr(unsafe.Pointer(utf16Ptr)))
}

// UpdateTooltip updates the text displayed when hovering over the tray icon.
func (tm *TrayManager) UpdateTooltip(tip string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.stopped || tm.hwnd == 0 {
		return
	}

	tm.nid.UFlags = NIF_TIP
	copyTip(&tm.nid.SzTip, tip)
	procShellNotifyIcon.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&tm.nid)))
}

// ShowBalloonNotification displays a native Windows notification balloon from the tray.
func (tm *TrayManager) ShowBalloonNotification(title, text string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.stopped || tm.hwnd == 0 {
		return
	}

	tm.nid.UFlags = NIF_INFO
	tm.nid.DwInfoFlags = NIIF_INFO
	copyTip256(&tm.nid.SzInfo, text)
	copyTip64(&tm.nid.SzInfoTitle, title)
	procShellNotifyIcon.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&tm.nid)))
}

// Destroy cleans up and removes the tray icon from the taskbar notification area.
func (tm *TrayManager) Destroy() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.stopped {
		return
	}
	tm.stopped = true
	if tm.hwnd != 0 {
		procShellNotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(&tm.nid)))
		procPostMessage.Call(tm.hwnd, 0x0010, 0, 0) // WM_CLOSE
	}
}

func copyTip(dest *[128]uint16, src string) {
	utf := syscall.StringToUTF16(src)
	for i := 0; i < len(dest); i++ {
		if i < len(utf) {
			dest[i] = utf[i]
		} else {
			dest[i] = 0
		}
	}
}

func copyTip256(dest *[256]uint16, src string) {
	utf := syscall.StringToUTF16(src)
	for i := 0; i < len(dest); i++ {
		if i < len(utf) {
			dest[i] = utf[i]
		} else {
			dest[i] = 0
		}
	}
}

func copyTip64(dest *[64]uint16, src string) {
	utf := syscall.StringToUTF16(src)
	for i := 0; i < len(dest); i++ {
		if i < len(utf) {
			dest[i] = utf[i]
		} else {
			dest[i] = 0
		}
	}
}

// ShowAppWindow restores and brings the main NeoDM window to the foreground.
func ShowAppWindow() {
	hwnd := GetAppHWND()
	if hwnd != 0 {
		procShowWindow.Call(hwnd, uintptr(SW_SHOW))
		procShowWindow.Call(hwnd, uintptr(SW_RESTORE))
		procSetForegroundWindow.Call(hwnd)
	}
}

// HideAppWindow hides the main NeoDM window to the system tray.
func HideAppWindow() {
	hwnd := GetAppHWND()
	if hwnd != 0 {
		procShowWindow.Call(hwnd, uintptr(SW_HIDE))
	}
}
