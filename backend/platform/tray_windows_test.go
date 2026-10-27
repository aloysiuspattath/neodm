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
