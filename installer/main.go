package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"ifd/backend/platform"
)

func main() {
	silent := flag.Bool("silent", false, "Perform unattended silent installation")
	flag.BoolVar(silent, "s", false, "Perform unattended silent installation")
	uninstall := flag.Bool("uninstall", false, "Uninstall NeoDM from the system")
	noLaunch := flag.Bool("no-launch", false, "Do not launch NeoDM after installation")
	flag.Parse()

	// If explicit uninstall flag passed
	if *uninstall {
		if !*silent {
			if !platform.ShowConfirmDialog("Uninstall NeoDM PRO", "Are you sure you want to completely uninstall Neo Download Manager PRO?") {
				os.Exit(0)
			}
		}
		killCmd := exec.Command("taskkill", "/F", "/IM", "neodm.exe")
		killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = killCmd.Run()
		killCmd2 := exec.Command("taskkill", "/F", "/IM", "ifd.exe")
		killCmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = killCmd2.Run()

		if err := platform.UninstallApp(); err != nil {
			if !*silent {
				platform.ShowInfoDialog("Uninstall Error", fmt.Sprintf("Error during uninstall: %v", err))
			}
			os.Exit(1)
		}
		if !*silent {
			platform.ShowInfoDialog("Uninstall Complete", "Neo Download Manager PRO has been completely removed from this computer.")
		}
		os.Exit(0)
	}

	isInstalled := platform.IsAppInstalled()
	isUpdate := isInstalled

	// Interactive mode: Check if already installed and prompt user for action
	if !*silent {
		action := platform.PromptInstallerAction(isInstalled)
		switch action {
		case "exit":
			os.Exit(0)
		case "uninstall":
			if !platform.ShowConfirmDialog("Confirm Uninstall", "Are you sure you want to uninstall Neo Download Manager PRO?") {
				os.Exit(0)
			}
			killCmd := exec.Command("taskkill", "/F", "/IM", "neodm.exe")
			killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			_ = killCmd.Run()
			killCmd2 := exec.Command("taskkill", "/F", "/IM", "ifd.exe")
			killCmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			_ = killCmd2.Run()

			if err := platform.UninstallApp(); err != nil {
				platform.ShowInfoDialog("Uninstall Error", fmt.Sprintf("Error during uninstall: %v", err))
				os.Exit(1)
			}
			platform.ShowInfoDialog("Uninstall Complete", "Neo Download Manager PRO has been uninstalled from your computer.")
			os.Exit(0)
		case "update":
			isUpdate = true
		case "install":
			isUpdate = false
		}
	}

	// Terminate any running instances first
	killCmd := exec.Command("taskkill", "/F", "/IM", "neodm.exe")
	killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = killCmd.Run()
	killCmd2 := exec.Command("taskkill", "/F", "/IM", "ifd.exe")
	killCmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = killCmd2.Run()
	time.Sleep(500 * time.Millisecond)

	err := platform.InstallApp()
	if err != nil {
		if !*silent {
			platform.ShowInfoDialog("Setup Error", fmt.Sprintf("Installation failed: %v", err))
		}
		os.Exit(1)
	}

	if !*silent {
		if isUpdate {
			platform.ShowInfoDialog("NeoDM Updated", "Neo Download Manager PRO has been successfully updated to the latest version!\n\nYour existing downloads, history, and settings have been preserved.")
		} else {
			platform.ShowInfoDialog("Setup Complete", "Neo Download Manager PRO has been installed successfully!\n\nDesktop shortcuts and browser companion extensions are ready.")
		}
	}

	// Launch installed app if requested
	if !*noLaunch {
		installedExe := filepath.Join(platform.GetInstallDir(), "neodm.exe")
		launchCmd := exec.Command(installedExe)
		launchCmd.Dir = platform.GetInstallDir()
		// Do NOT set HideWindow: true on GUI application, as it passes SW_HIDE into STARTUPINFO
		// causing WebView2 compositor to fail surface creation (blank screen).
		_ = launchCmd.Start()
	}
}
