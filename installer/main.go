package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"ifd/backend/platform"
)

// Embedded application binaries & resources for true single-file standalone installer
//
//go:embed neodm.exe
var embeddedApp []byte

//go:embed app.ico
var embeddedIco []byte

//go:embed extension/*
var embeddedExt embed.FS

func main() {
	var silent bool
	var uninstall bool
	var noLaunch bool

	// Parse custom / Windows-style CLI switches (/S, /silent, /quiet, /qn, /uninstall, /no-launch)
	for _, arg := range os.Args[1:] {
		cleaned := strings.ToLower(strings.TrimLeft(arg, "-/"))
		switch cleaned {
		case "s", "silent", "quiet", "qn", "verysilent":
			silent = true
		case "uninstall", "remove":
			uninstall = true
		case "no-launch", "nolaunch", "no_launch":
			noLaunch = true
		}
	}

	// Also support Go standard flag syntax (-silent, -s, -uninstall, -no-launch)
	flagSilent := flag.Bool("silent", false, "Perform unattended silent installation")
	flag.BoolVar(flagSilent, "s", false, "Perform unattended silent installation")
	flagUninstall := flag.Bool("uninstall", false, "Uninstall NeoDM from the system")
	flagNoLaunch := flag.Bool("no-launch", false, "Do not launch NeoDM after installation")
	_ = flag.CommandLine.Parse(os.Args[1:])

	if *flagSilent {
		silent = true
	}
	if *flagUninstall {
		uninstall = true
	}
	if *flagNoLaunch {
		noLaunch = true
	}

	// Detect non-interactive CI / sandbox environments (Azure Pipelines, GitHub Actions, WinGet Sandbox)
	if os.Getenv("CI") != "" || os.Getenv("TF_BUILD") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		silent = true
	}

	// 1. Uninstall mode
	if uninstall {
		if !silent {
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
			if !silent {
				platform.ShowInfoDialog("Uninstall Error", fmt.Sprintf("Error during uninstall: %v", err))
			}
			os.Exit(1)
		}
		if !silent {
			platform.ShowInfoDialog("Uninstall Complete", "Neo Download Manager PRO has been completely removed from this computer.")
		}
		os.Exit(0)
	}

	isInstalled := platform.IsAppInstalled()
	isUpdate := isInstalled

	// 2. Interactive prompt (only when NOT silent)
	if !silent {
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

	// 3. Perform Installation using embedded payload
	err := platform.InstallAppWithEmbedded(embeddedApp, embeddedIco, embeddedExt)
	if err != nil {
		if !silent {
			platform.ShowInfoDialog("Setup Error", fmt.Sprintf("Installation failed: %v", err))
		} else {
			fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
		}
		os.Exit(1)
	}

	if !silent {
		if isUpdate {
			platform.ShowInfoDialog("NeoDM Updated", "Neo Download Manager PRO has been successfully updated to the latest version!\n\nYour existing downloads, history, and settings have been preserved.")
		} else {
			platform.ShowInfoDialog("Setup Complete", "Neo Download Manager PRO has been installed successfully!\n\nDesktop shortcuts and browser companion extensions are ready.")
		}
	}

	// 4. Launch installed app if requested
	if !noLaunch && !silent {
		installedExe := filepath.Join(platform.GetInstallDir(), "neodm.exe")
		launchCmd := exec.Command(installedExe)
		launchCmd.Dir = platform.GetInstallDir()
		_ = launchCmd.Start()
	}

	os.Exit(0)
}
