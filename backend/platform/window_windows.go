//go:build windows
// +build windows

package platform

import (
	"sync"
	"syscall"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procShowWindow       = user32.NewProc("ShowWindow")
	procIsZoomed         = user32.NewProc("IsZoomed")
	procPostMessage      = user32.NewProc("PostMessageW")
	procSendMessage      = user32.NewProc("SendMessageW")
	procReleaseCapture   = user32.NewProc("ReleaseCapture")
	procGetWindowLong    = user32.NewProc("GetWindowLongW")
	procSetWindowLong    = user32.NewProc("SetWindowLongW")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")

	appHWND uintptr
	hwndMu  sync.RWMutex
)

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

	style, _, _ := procGetWindowLong.Call(hwnd, uintptr(GWL_STYLE))
	// Remove standard title bar caption but keep resize frame and minimize/maximize support
	newStyle := style &^ WS_CAPTION
	procSetWindowLong.Call(hwnd, uintptr(GWL_STYLE), newStyle)
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER|SWP_FRAMECHANGED)
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
		procSendMessage.Call(hwnd, uintptr(WM_SYSCOMMAND), uintptr(SC_DRAGMOVE), 0)
		procPostMessage.Call(hwnd, uintptr(WM_NCLBUTTONDOWN), uintptr(HTCAPTION), 0)
	}
}
