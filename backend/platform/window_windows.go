//go:build windows
// +build windows

package platform

import (
	"os"
	"path/filepath"
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
	procSetClassLongPtr     = user32.NewProc("SetClassLongPtrW")
	procSetAppUserModelID   = shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")

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
	AppUserModelID = "NeoDM.DownloadManager.Pro.1.0"

	GCLP_HICON   = ^uintptr(13) // -14
	GCLP_HICONSM = ^uintptr(33) // -34
)

// SetProcessAppID binds the Windows Taskbar AppUserModelID for explicit icon grouping and notifications.
func SetProcessAppID() {
	appIDPtr := syscall.StringToUTF16Ptr(AppUserModelID)
	procSetAppUserModelID.Call(uintptr(unsafe.Pointer(appIDPtr)))
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

	// 1. Try loading 32x32 (big) and 16x16 (small) icon from embedded Win32 resource ID 1
	hIconBig, _, _ := procLoadImage.Call(hInstance, uintptr(1), uintptr(IMAGE_ICON), 32, 32, uintptr(LR_SHARED))
	hIconSmall, _, _ := procLoadImage.Call(hInstance, uintptr(1), uintptr(IMAGE_ICON), 16, 16, uintptr(LR_SHARED))

	// 2. Try loading from app.ico or frontend/favicon.ico on disk if running unpackaged/dev
	if hIconBig == 0 || hIconSmall == 0 {
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
				if hIconBig == 0 {
					hIconBig, _, _ = procLoadImage.Call(0, uintptr(unsafe.Pointer(pUtf16)), uintptr(IMAGE_ICON), 32, 32, uintptr(LR_LOADFROMFILE))
				}
				if hIconSmall == 0 {
					hIconSmall, _, _ = procLoadImage.Call(0, uintptr(unsafe.Pointer(pUtf16)), uintptr(IMAGE_ICON), 16, 16, uintptr(LR_LOADFROMFILE))
				}
				if hIconBig != 0 && hIconSmall != 0 {
					break
				}
			}
		}
	}

	// 3. Fallback to ExtractIconW from running executable
	if hIconBig == 0 || hIconBig == 1 {
		exePathBuf := make([]uint16, 1024)
		procGetModuleFileName.Call(0, uintptr(unsafe.Pointer(&exePathBuf[0])), uintptr(len(exePathBuf)))
		hIconBig, _, _ = procExtractIcon.Call(hInstance, uintptr(unsafe.Pointer(&exePathBuf[0])), 0)
	}

	// 4. Fallback to standard application icon
	if hIconBig == 0 || hIconBig == 1 {
		hIconBig, _, _ = procLoadIcon.Call(0, uintptr(IDI_APPLICATION))
	}
	if hIconSmall == 0 || hIconSmall == 1 {
		hIconSmall = hIconBig
	}

	// Apply icons to window instance and window class
	if hIconSmall != 0 && hIconSmall != 1 {
		procSendMessage.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_SMALL), hIconSmall)
		procSetClassLongPtr.Call(hwnd, GCLP_HICONSM, hIconSmall)
	}
	if hIconBig != 0 && hIconBig != 1 {
		procSendMessage.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_BIG), hIconBig)
		procSetClassLongPtr.Call(hwnd, GCLP_HICON, hIconBig)
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
