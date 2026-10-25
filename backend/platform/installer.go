package platform

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const uninstallRegKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\NeoDM`

// GetInstallDir returns default user-level installation path: %LOCALAPPDATA%\Programs\NeoDM
func GetInstallDir() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "AppData", "Local", "Programs", "NeoDM")
	}
	return filepath.Join(localAppData, "Programs", "NeoDM")
}

// CreateShortcut creates a Windows .lnk shortcut via WScript.Shell COM
func CreateShortcut(targetExe, shortcutPath, desc string) error {
	_ = os.MkdirAll(filepath.Dir(shortcutPath), 0755)

	psScript := fmt.Sprintf(`
$ws = New-Object -ComObject WScript.Shell
$s = $ws.CreateShortcut('%s')
$s.TargetPath = '%s'
$s.WorkingDirectory = '%s'
$s.Description = '%s'
$s.Save()
`, shortcutPath, targetExe, filepath.Dir(targetExe), desc)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.Run()
}

// InstallApp deploys IFD, creates shortcuts, and registers with Windows Add/Remove Programs
func InstallApp() error {
	installDir := GetInstallDir()
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("failed to create install dir: %w", err)
	}

	// 1. Determine source executable
	currentExe, err := os.Executable()
	if err != nil {
		return err
	}
	sourceExe := currentExe
	if strings.EqualFold(filepath.Base(currentExe), "NeoDM-Setup.exe") || strings.EqualFold(filepath.Base(currentExe), "IFD-Setup.exe") {
		candidate := filepath.Join(filepath.Dir(currentExe), "neodm.exe")
		if _, err := os.Stat(candidate); err == nil {
			sourceExe = candidate
		} else {
			candidateIFD := filepath.Join(filepath.Dir(currentExe), "ifd.exe")
			if _, err := os.Stat(candidateIFD); err == nil {
				sourceExe = candidateIFD
			}
		}
	}

	targetExe := filepath.Join(installDir, "neodm.exe")

	if filepath.Clean(sourceExe) != filepath.Clean(targetExe) {
		if err := copyFile(sourceExe, targetExe); err != nil {
			return fmt.Errorf("failed to copy executable: %w", err)
		}
	}

	// 2. Copy extension directory if available next to source executable
	sourceExtDir := filepath.Join(filepath.Dir(currentExe), "extension")
	targetExtDir := filepath.Join(installDir, "extension")
	if _, err := os.Stat(sourceExtDir); err == nil {
		_ = copyDir(sourceExtDir, targetExtDir)
	}

	// 3. Create Start Menu shortcut
	appData := os.Getenv("APPDATA")
	startMenuDir := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs")
	startMenuLnk := filepath.Join(startMenuDir, "Neo Download Manager PRO.lnk")
	_ = CreateShortcut(targetExe, startMenuLnk, "Neo Download Manager PRO - High-Speed Accelerator")

	// 4. Create Desktop shortcut
	userProfile := os.Getenv("USERPROFILE")
	desktopLnk := filepath.Join(userProfile, "Desktop", "Neo Download Manager PRO.lnk")
	_ = CreateShortcut(targetExe, desktopLnk, "Neo Download Manager PRO - High-Speed Accelerator")

	// 5. Register in Windows Registry (Add or Remove Programs)
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallRegKey, registry.SET_VALUE)
	if err == nil {
		defer k.Close()
		_ = k.SetStringValue("DisplayName", "Neo Download Manager PRO (NeoDM)")
		_ = k.SetStringValue("DisplayVersion", "1.0.0")
		_ = k.SetStringValue("Publisher", "NeoDM Team")
		_ = k.SetStringValue("InstallLocation", installDir)
		_ = k.SetStringValue("DisplayIcon", targetExe)
		_ = k.SetStringValue("UninstallString", fmt.Sprintf(`"%s" --uninstall`, targetExe))
		_ = k.SetDWordValue("NoModify", 1)
		_ = k.SetDWordValue("NoRepair", 1)
	}

	return nil
}

// UninstallApp removes shortcuts, registry keys, and program directory
func UninstallApp() error {
	// 1. Remove Registry Key
	_ = registry.DeleteKey(registry.CURRENT_USER, uninstallRegKey)

	// 2. Remove Startup entry
	_ = SetAutoStart(false)

	// 3. Remove Start Menu & Desktop shortcuts
	appData := os.Getenv("APPDATA")
	startMenuDir := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs")
	_ = os.Remove(filepath.Join(startMenuDir, "Neo Download Manager PRO.lnk"))
	_ = os.Remove(filepath.Join(startMenuDir, "IFD - Internet Fast Downloader.lnk"))

	userProfile := os.Getenv("USERPROFILE")
	desktopDir := filepath.Join(userProfile, "Desktop")
	_ = os.Remove(filepath.Join(desktopDir, "Neo Download Manager PRO.lnk"))
	_ = os.Remove(filepath.Join(desktopDir, "IFD.lnk"))

	// 4. Schedule directory removal after exit via PowerShell
	installDir := GetInstallDir()
	psScript := fmt.Sprintf(`Start-Sleep -Seconds 1; Remove-Item -Recurse -Force '%s'`, installDir)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	_ = os.MkdirAll(dst, 0755)
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		s := filepath.Join(src, entry.Name())
		d := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			_ = copyDir(s, d)
		} else {
			_ = copyFile(s, d)
		}
	}
	return nil
}
