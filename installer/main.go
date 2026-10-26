package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"ifd/backend/platform"
)

func main() {
	silent := flag.Bool("silent", false, "Perform unattended silent installation")
	flag.BoolVar(silent, "s", false, "Perform unattended silent installation")
	noLaunch := flag.Bool("no-launch", false, "Do not launch NeoDM after installation")
	flag.Parse()

	if !*silent {
		fmt.Println("==================================================")
		fmt.Println("   Neo Download Manager PRO (NeoDM) - Setup       ")
		fmt.Println("==================================================")
		fmt.Println("Installing NeoDM to:", platform.GetInstallDir())
		fmt.Println("Deploying components and creating shortcuts...")
	}

	// Terminate any running instances first
	killCmd := exec.Command("taskkill", "/F", "/IM", "neodm.exe")
	killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = killCmd.Run()
	killCmd2 := exec.Command("taskkill", "/F", "/IM", "ifd.exe")
	killCmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = killCmd2.Run()

	err := platform.InstallApp()
	if err != nil {
		fmt.Printf("Installation error: %v\n", err)
		if !*silent {
			fmt.Println("Press Enter to exit...")
			var dummy string
			fmt.Scanln(&dummy)
		}
		os.Exit(1)
	}

	if !*silent {
		fmt.Println("✓ Neo Download Manager PRO installed successfully!")
		fmt.Println("✓ Desktop shortcut created")
		fmt.Println("✓ Start Menu shortcut created")
		fmt.Println("✓ Browser Companion Extension installed & registered (Chrome, Edge, Brave, Firefox)")
		fmt.Println("✓ Windows Add/Remove Programs entry & uninstaller (uninstall.exe) registered")
	}

	// Launch installed app if requested
	if !*noLaunch {
		if !*silent {
			fmt.Println("Launching Neo Download Manager PRO...")
		}
		installedExe := filepath.Join(platform.GetInstallDir(), "neodm.exe")
		if _, err := os.Stat(installedExe); os.IsNotExist(err) {
			installedExe = filepath.Join(platform.GetInstallDir(), "ifd.exe")
		}
		launchCmd := exec.Command(installedExe)
		launchCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = launchCmd.Start()
	}
}
