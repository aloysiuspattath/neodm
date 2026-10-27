//go:build !windows
// +build !windows

package platform

func RegisterAppHWND(hwnd uintptr) {}
func GetAppHWND() uintptr { return 0 }
func SetupFramelessWindow(hwnd uintptr) {}
func PerformWindowAction(hwnd uintptr, action string) {}
func DetachConsole() {}
func RestartApp() {}
