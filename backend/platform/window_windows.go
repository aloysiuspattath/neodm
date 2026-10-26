//go:build windows
// +build windows

package platform

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	procFreeConsole      = kernel32.NewProc("FreeConsole")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")

	user32                  = syscall.NewLazyDLL("user32.dll")
	procShowWindow          = user32.NewProc("ShowWindow")
	procIsZoomed            = user32.NewProc("IsZoomed")
	procPostMessage         = user32.NewProc("PostMessageW")
	procSendMessage         = user32.NewProc("SendMessageW")
	procReleaseCapture      = user32.NewProc("ReleaseCapture")
	procGetWindowLong       = user32.NewProc("GetWindowLongW")
	procSetWindowLong       = user32.NewProc("SetWindowLongW")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")

	dwmapi                    = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

	appHWND uintptr
	hwndMu  sync.RWMutex
)

const (
	DWMWA_USE_IMMERSIVE_DARK_MODE     = 20
	DWMWA_USE_IMMERSIVE_DARK_MODE_OLD = 19
	DWMWA_BORDER_COLOR                = 34
	DWMWA_CAPTION_COLOR               = 35
)

// DetachConsole immediately hides and frees any console window when in GUI mode.
func DetachConsole() {
	if hwnd, _, _ := procGetConsoleWindow.Call(); hwnd != 0 {
		procShowWindow.Call(hwnd, 0) // SW_HIDE
	}
	procFreeConsole.Call()
}

const (
	GWL_STYLE          = 0xFFFFFFF0 // -16 in 32-bit two's complement
	WS_CAPTION         = 0x00C00000
	WS_THICKFRAME      = 0x00040000
	WS_MINIMIZEBOX     = 0x00020000
	WS_MAXIMIZEBOX     = 0x00010000

	SWP_NOSIZE       = 0x0001
	SWP_NOMOVE       = 0x0002
	SWP_NOZORDER     = 0x0004
	SWP_FRAMECHANGED = 0x0020

	SW_MINIMIZE = 6
	SW_MAXIMIZE = 3
	SW_RESTORE  = 9

	WM_CLOSE         = 0x0010
	WM_SYSCOMMAND    = 0x0112
	SC_MOVE          = 0xF010
	SC_DRAGMOVE      = 0xF012 // SC_MOVE | 2 initiates native window drag on any window style
	WM_NCLBUTTONDOWN = 0x00A1
	HTCAPTION        = 2

	WM_SETICON = 0x0080
	ICON_SMALL = 0
	ICON_BIG   = 1
)

// RegisterAppHWND sets the active window handle for IFD.
func RegisterAppHWND(hwnd uintptr) {
	hwndMu.Lock()
	defer hwndMu.Unlock()
	appHWND = hwnd
}

// GetAppHWND returns the stored window handle.
func GetAppHWND() uintptr {
	hwndMu.RLock()
	defer hwndMu.RUnlock()
	if appHWND != 0 {
		return appHWND
	}
	r, _, _ := procGetForegroundWindow.Call()
	return r
}

// SetupFramelessWindow removes the OS caption while preserving resize borders and DWM drop shadow.
func SetupFramelessWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	RegisterAppHWND(hwnd)

	// Apply Windows 10/11 immersive dark mode to window frame and non-client border
	darkMode := int32(1)
	procDwmSetWindowAttribute.Call(hwnd, uintptr(DWMWA_USE_IMMERSIVE_DARK_MODE), uintptr(unsafe.Pointer(&darkMode)), 4)
	procDwmSetWindowAttribute.Call(hwnd, uintptr(DWMWA_USE_IMMERSIVE_DARK_MODE_OLD), uintptr(unsafe.Pointer(&darkMode)), 4)

	// Set border and caption color to dark slate (#0b0f19 -> 0x00190F0B in COLORREF format) to eliminate bright white frame line
	darkBorder := uint32(0x00190F0B)
	procDwmSetWindowAttribute.Call(hwnd, uintptr(DWMWA_BORDER_COLOR), uintptr(unsafe.Pointer(&darkBorder)), 4)
	procDwmSetWindowAttribute.Call(hwnd, uintptr(DWMWA_CAPTION_COLOR), uintptr(unsafe.Pointer(&darkBorder)), 4)

	style, _, _ := procGetWindowLong.Call(hwnd, uintptr(GWL_STYLE))
	// Remove standard title bar caption but keep resize frame and minimize/maximize support
	newStyle := style &^ WS_CAPTION
	procSetWindowLong.Call(hwnd, uintptr(GWL_STYLE), newStyle)
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER|SWP_FRAMECHANGED)

	// Set explicit application icon on the window for taskbar, Alt+Tab, and titlebar
	hInstance, _, _ := procGetModuleHandle.Call(0)
	hIcon, _, _ := procLoadIcon.Call(hInstance, uintptr(1))
	if hIcon == 0 || hIcon == 1 {
		hIcon, _, _ = procLoadIcon.Call(0, uintptr(IDI_APPLICATION))
	}
	if hIcon != 0 && hIcon != 1 {
		procSendMessage.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_SMALL), hIcon)
		procSendMessage.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_BIG), hIcon)
	}
}

// PerformWindowAction handles window minimize, maximize/restore, close, and drag.
func PerformWindowAction(hwnd uintptr, action string) {
	if hwnd == 0 {
		hwnd = GetAppHWND()
	}
	if hwnd == 0 {
		return
	}

	switch action {
	case "minimize":
		procShowWindow.Call(hwnd, uintptr(SW_MINIMIZE))
	case "hide":
		procShowWindow.Call(hwnd, uintptr(SW_HIDE))
	case "show":
		procShowWindow.Call(hwnd, uintptr(SW_SHOW))
		procShowWindow.Call(hwnd, uintptr(SW_RESTORE))
		procSetForegroundWindow.Call(hwnd)
	case "maximize":
		isMaximized, _, _ := procIsZoomed.Call(hwnd)
		if isMaximized != 0 {
			procShowWindow.Call(hwnd, uintptr(SW_RESTORE))
		} else {
			procShowWindow.Call(hwnd, uintptr(SW_MAXIMIZE))
		}
	case "close":
		// If system tray is active, minimize to tray seamlessly
		if GetGlobalTray() != nil {
			procShowWindow.Call(hwnd, uintptr(SW_HIDE))
		} else {
			procPostMessage.Call(hwnd, uintptr(WM_CLOSE), 0, 0)
		}
	case "quit", "exit":
		procPostMessage.Call(hwnd, uintptr(WM_CLOSE), 0, 0)
	case "drag":
		procReleaseCapture.Call()
		procPostMessage.Call(hwnd, uintptr(WM_NCLBUTTONDOWN), uintptr(HTCAPTION), 0)
		procPostMessage.Call(hwnd, uintptr(WM_SYSCOMMAND), uintptr(SC_DRAGMOVE), 0)
	}
}
