//go:build windows
// +build windows

package platform

import (
	"os"
	"testing"
	"syscall"
	"unsafe"
)

func TestIconExtractionAndLoading(t *testing.T) {
	hInstance, _, _ := procGetModuleHandle.Call(0)
	if hInstance == 0 {
		t.Fatalf("procGetModuleHandle returned 0")
	}

	// Verify app.ico exists on disk
	if fi, err := os.Stat("../../app.ico"); err != nil || fi.Size() == 0 {
		t.Logf("app.ico not found at relative path: %v", err)
	} else {
		t.Logf("app.ico found, size: %d bytes", fi.Size())
	}

	// Test ExtractAssociatedIcon from current running test executable or neodm.exe
	targetExe := "../../neodm.exe"
	if _, err := os.Stat(targetExe); err == nil {
		pExe, err := syscall.UTF16PtrFromString(targetExe)
		if err == nil {
			hIcon, _, _ := procExtractIcon.Call(hInstance, uintptr(unsafe.Pointer(pExe)), 0)
			if hIcon != 0 && hIcon != 1 {
				t.Logf("Successfully extracted icon handle: %x from neodm.exe", hIcon)
			} else {
				t.Logf("ExtractIcon returned %x", hIcon)
			}
		}
	}
}

func TestWindowStyleAndFrameless(t *testing.T) {
	hInstance, _, _ := procGetModuleHandle.Call(0)
	className, _ := syscall.UTF16PtrFromString("TestWindowClass")
	wndClass := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   procDefWindowProc.Addr(),
		HInstance:     hInstance,
		LpszClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wndClass)))

	hwnd, _, _ := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		0xCF0000, // WS_OVERLAPPEDWINDOW
		0, 0, 800, 600,
		0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		t.Fatalf("CreateWindowEx failed")
	}
	defer procDestroyWindow.Call(hwnd)

	// Test GetWindowLongW with GWL_STYLE
	styleOld, _, _ := procGetWindowLong.Call(hwnd, uintptr(GWL_STYLE))
	styleSigned, _, _ := procGetWindowLong.Call(hwnd, ^uintptr(15)) // -16
	t.Logf("style with GWL_STYLE (0xFFFFFFF0): %x", styleOld)
	t.Logf("style with ^uintptr(15) (-16): %x", styleSigned)

	if styleOld == 0 && styleSigned != 0 {
		t.Errorf("GWL_STYLE 0xFFFFFFF0 FAILED! Must use ^uintptr(15) (-16) in 64-bit!")
	}
}
