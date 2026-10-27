package platform

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const (
	uninstallRegKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\NeoDM`
	extensionID     = "neodmdownloadmanagerproaccelerat"
	chromeExtKey    = `Software\Google\Chrome\Extensions\` + extensionID
	edgeExtKey      = `Software\Microsoft\Edge\Extensions\` + extensionID
	braveExtKey     = `Software\BraveSoftware\Brave-Browser\Extensions\` + extensionID
	firefoxExtKey   = `Software\Mozilla\Firefox\Extensions`
)

var (
	user32DLL       = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32DLL.NewProc("MessageBoxW")
)

const (
	MB_OK              = 0x00000000
	MB_YESNO           = 0x00000004
	MB_ICONQUESTION    = 0x00000020
	MB_ICONINFORMATION = 0x00000040
	IDYES              = 6
)

// ShowConfirmDialog displays a native Windows confirmation dialog (Yes/No)
func ShowConfirmDialog(title, message string) bool {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(message)
	ret, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), uintptr(MB_YESNO|MB_ICONQUESTION))
	return ret == IDYES
}

// ShowInfoDialog displays a native Windows info message box
func ShowInfoDialog(title, message string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(message)
	_, _, _ = procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), uintptr(MB_OK|MB_ICONINFORMATION))
}

// GetInstallDir returns default user-level installation path: %LOCALAPPDATA%\Programs\NeoDM
func GetInstallDir() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "AppData", "Local", "Programs", "NeoDM")
	}
	return filepath.Join(localAppData, "Programs", "NeoDM")
}

// GetExtensionDir finds the installed or local extension directory
func GetExtensionDir() string {
	// 1. Next to current running executable
	if exe, err := os.Executable(); err == nil {
		ext := filepath.Join(filepath.Dir(exe), "extension")
		if fi, err := os.Stat(ext); err == nil && fi.IsDir() {
			return ext
		}
	}
	// 2. In install dir
	instExt := filepath.Join(GetInstallDir(), "extension")
	if fi, err := os.Stat(instExt); err == nil && fi.IsDir() {
		return instExt
	}
	// 3. Fallback to current working directory
	if fi, err := os.Stat("extension"); err == nil && fi.IsDir() {
		if abs, err := filepath.Abs("extension"); err == nil {
			return abs
		}
	}
	return ""
}

// RegisterBrowserExtensions registers extension paths with Chrome, Edge, Brave, and Firefox
func RegisterBrowserExtensions(extDir string) error {
	if extDir == "" {
		extDir = GetExtensionDir()
	}
	if extDir == "" {
		return fmt.Errorf("extension directory not found")
	}

	manifestPath := filepath.Join(extDir, "manifest.json")
	if _, err := os.Stat(manifestPath); err != nil {
		return fmt.Errorf("manifest.json not found in %s", extDir)
	}

	// 1. Chrome
	if k, _, err := registry.CreateKey(registry.CURRENT_USER, chromeExtKey, registry.SET_VALUE); err == nil {
		_ = k.SetStringValue("path", extDir)
		_ = k.SetStringValue("version", "1.0.0")
		k.Close()
	}

	// 2. Microsoft Edge
	if k, _, err := registry.CreateKey(registry.CURRENT_USER, edgeExtKey, registry.SET_VALUE); err == nil {
		_ = k.SetStringValue("path", extDir)
		_ = k.SetStringValue("version", "1.0.0")
		k.Close()
	}

	// 3. Brave
	if k, _, err := registry.CreateKey(registry.CURRENT_USER, braveExtKey, registry.SET_VALUE); err == nil {
		_ = k.SetStringValue("path", extDir)
		_ = k.SetStringValue("version", "1.0.0")
		k.Close()
	}

	// 4. Firefox
	if k, _, err := registry.CreateKey(registry.CURRENT_USER, firefoxExtKey, registry.SET_VALUE); err == nil {
		_ = k.SetStringValue("neodm-extension@neodm.io", extDir)
		k.Close()
	}

	return nil
}

// UnregisterBrowserExtensions removes browser extension registry entries
func UnregisterBrowserExtensions() {
	_ = registry.DeleteKey(registry.CURRENT_USER, chromeExtKey)
	_ = registry.DeleteKey(registry.CURRENT_USER, edgeExtKey)
	_ = registry.DeleteKey(registry.CURRENT_USER, braveExtKey)
	if k, err := registry.OpenKey(registry.CURRENT_USER, firefoxExtKey, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue("neodm-extension@neodm.io")
		k.Close()
	}
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
$s.IconLocation = '%s,0'
$s.Save()
`, shortcutPath, targetExe, filepath.Dir(targetExe), desc, targetExe)

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
	if strings.Contains(strings.ToLower(filepath.Base(currentExe)), "setup") {
		candidate := filepath.Join(filepath.Dir(currentExe), "neodm.exe")
		if _, err := os.Stat(candidate); err == nil {
			sourceExe = candidate
		}
	}

	targetExe := filepath.Join(installDir, "neodm.exe")

	if filepath.Clean(sourceExe) != filepath.Clean(targetExe) {
		if err := copyFile(sourceExe, targetExe); err != nil {
			return fmt.Errorf("failed to copy executable: %w", err)
		}
	}
	_ = copyFile(targetExe, filepath.Join(installDir, "uninstall.exe"))

	// Copy app.ico alongside executable for direct filesystem fallbacks
	sourceIco := filepath.Join(filepath.Dir(currentExe), "app.ico")
	if _, err := os.Stat(sourceIco); os.IsNotExist(err) {
		sourceIco = "app.ico"
	}
	if _, err := os.Stat(sourceIco); err == nil {
		_ = copyFile(sourceIco, filepath.Join(installDir, "app.ico"))
	}

	// 2. Copy extension directory if available next to source executable
	sourceExtDir := filepath.Join(filepath.Dir(currentExe), "extension")
	if _, err := os.Stat(sourceExtDir); os.IsNotExist(err) {
		sourceExtDir = "extension"
	}
	targetExtDir := filepath.Join(installDir, "extension")
	if fi, err := os.Stat(sourceExtDir); err == nil && fi.IsDir() {
		_ = copyDir(sourceExtDir, targetExtDir)
		_ = RegisterBrowserExtensions(targetExtDir)
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
		_ = k.SetStringValue("UninstallString", fmt.Sprintf(`"%s"`, filepath.Join(installDir, "uninstall.exe")))
		_ = k.SetStringValue("QuietUninstallString", fmt.Sprintf(`"%s" --silent`, filepath.Join(installDir, "uninstall.exe")))
		_ = k.SetDWordValue("NoModify", 1)
		_ = k.SetDWordValue("NoRepair", 1)
	}

	return nil
}

// UninstallApp removes shortcuts, registry keys, and program directory
func UninstallApp() error {
	// 1. Unregister Browser Extensions
	UnregisterBrowserExtensions()

	// 2. Remove Registry Key
	_ = registry.DeleteKey(registry.CURRENT_USER, uninstallRegKey)

	// 3. Remove Startup entry
	_ = SetAutoStart(false)

	// 4. Remove Start Menu & Desktop shortcuts
	appData := os.Getenv("APPDATA")
	startMenuDir := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs")
	_ = os.Remove(filepath.Join(startMenuDir, "Neo Download Manager PRO.lnk"))
	_ = os.Remove(filepath.Join(startMenuDir, "IFD - Internet Fast Downloader.lnk"))

	userProfile := os.Getenv("USERPROFILE")
	desktopDir := filepath.Join(userProfile, "Desktop")
	_ = os.Remove(filepath.Join(desktopDir, "Neo Download Manager PRO.lnk"))
	_ = os.Remove(filepath.Join(desktopDir, "IFD.lnk"))

	// 5. Schedule directory removal after exit via PowerShell
	installDir := GetInstallDir()
	psScript := fmt.Sprintf(`Start-Sleep -Seconds 2; Remove-Item -Recurse -Force '%s'`, installDir)
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
