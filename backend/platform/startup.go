package platform

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
const appName = "NeoDM"

// SetAutoStart configures NeoDM to launch at Windows login.
func SetAutoStart(enable bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if enable {
		exePath, err := os.Executable()
		if err != nil {
			return err
		}
		absPath, err := filepath.Abs(exePath)
		if err != nil {
			return err
		}
		cmd := `"` + absPath + `" -minimized`
		return k.SetStringValue(appName, cmd)
	}

	// Disable auto-start
	_ = k.DeleteValue(appName)
	return nil
}

// IsAutoStartEnabled checks if NeoDM is registered in Windows Startup.
func IsAutoStartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	val, _, err := k.GetStringValue(appName)
	return err == nil && val != ""
}
