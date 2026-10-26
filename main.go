package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"time"

	"ifd/backend/cli"
	"ifd/backend/engine"
	"ifd/backend/platform"
	"ifd/backend/server"

	"github.com/jchv/go-webview2"
)

//go:embed frontend/*
var frontendFS embed.FS

func main() {
	// Check for CLI execution before initializing GUI or WebView2
	if cli.RunCLI(os.Args) {
		return
	}

	// Immediately detach and hide any attached console window in GUI mode
	platform.DetachConsole()

	minimized := flag.Bool("minimized", false, "Start minimized in system tray")
	port := flag.Int("port", 19765, "Local HTTP/WebSocket server port")
	noGui := flag.Bool("nogui", false, "Run in headless daemon mode")
	installFlag := flag.Bool("install", false, "Install IFD, create desktop/start shortcuts, and register uninstaller")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall IFD and remove shortcuts")
	flag.Parse()

	if *installFlag {
		cli.AttachConsoleIfNeeded()
		if err := platform.InstallApp(); err != nil {
			log.Fatalf("Installation failed: %v", err)
		}
		fmt.Println("IFD installed successfully! Shortcuts created.")
		return
	}

	if *uninstallFlag {
		cli.AttachConsoleIfNeeded()
		if err := platform.UninstallApp(); err != nil {
			log.Fatalf("Uninstallation failed: %v", err)
		}
		fmt.Println("IFD uninstalled successfully.")
		return
	}

	// 1. Initialize persistent storage
	store, err := engine.NewStore()
	if err != nil {
		log.Fatalf("Failed to initialize store: %v", err)
	}

	// 2. Initialize download engine
	eng := engine.NewEngine(store)

	// Set completion alerts and notifications
	eng.SetCallbacks(nil, func(t *engine.Task) {
		if store.Settings.SoundAlertOnComplete {
			fmt.Print("\a") // Standard alert chime
		}
		if tray := platform.GetGlobalTray(); tray != nil {
			tray.ShowBalloonNotification("Nova Download Manager PRO", fmt.Sprintf("%s finished downloading.", t.Filename))
		} else {
			platform.ShowNotification("Nova Download Manager PRO", fmt.Sprintf("%s finished downloading.", t.Filename))
		}
	})

	// 3. Prepare frontend assets
	subFS, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		log.Fatalf("Failed to load embedded frontend: %v", err)
	}

	// 4. Start local daemon server
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	srv := server.NewServer(eng, store, subFS, addr)

	// Listen on port
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// Port may already be in use by an existing IFD instance
		log.Printf("Port %s already active: %v", addr, err)
		// If another instance is running, bring existing window to front or exit
		if !*noGui {
			platform.ShowAppWindow()
		}
		return
	}

	go func() {
		if err := srv.StartWithListener(listener); err != nil {
			log.Printf("Server stopped: %v", err)
		}
	}()

	// 5. Initialize desktop system tray
	var activeWebView webview2.WebView
	var tray *platform.TrayManager

	tray, _ = platform.NewTrayManager(platform.TrayCallbacks{
		OnShow: func() {
			platform.ShowAppWindow()
		},
		OnStartAll: func() {
			for _, rec := range eng.GetAllTasks() {
				if rec.Status == "Paused" || rec.Status == "Error" || rec.Status == "Queued" {
					if t := eng.GetTask(rec.ID); t != nil {
						t.Resume()
					}
				}
			}
		},
		OnPauseAll: func() {
			for _, rec := range eng.GetAllTasks() {
				if rec.Status == "Downloading" || rec.Status == "Connecting" {
					if t := eng.GetTask(rec.ID); t != nil {
						t.Pause()
					}
				}
			}
		},
		OnAddDownload: func() {
			platform.ShowAppWindow()
			if activeWebView != nil {
				activeWebView.Eval("openModal('modal-add-download'); switchAddTab('tab-add-url');")
			}
		},
		OnSettings: func() {
			platform.ShowAppWindow()
			if activeWebView != nil {
				activeWebView.Eval("openSettingsModal();")
			}
		},
		OnExit: func() {
			if tray != nil {
				tray.Destroy()
			}
			os.Exit(0)
		},
	})
	defer func() {
		if tray != nil {
			tray.Destroy()
		}
	}()

	eng.OnTaskUpdate(func(t *engine.Task) {
		if tray == nil {
			return
		}
		tasks := eng.GetAllTasks()
		activeCount := 0
		var totalSpeed int64 = 0
		for _, task := range tasks {
			if task.Status == "Downloading" {
				activeCount++
				totalSpeed += task.Speed
			}
		}
		if activeCount > 0 {
			speedStr := formatSpeed(totalSpeed)
			tray.UpdateTooltip(fmt.Sprintf("NovaDM - %d downloading (%s)", activeCount, speedStr))
		} else {
			tray.UpdateTooltip("Nova Download Manager PRO - Ready")
		}
	})

	// 6. Start clipboard monitoring
	clipboardWatcher := platform.NewClipboardWatcher(func(detectedURL string) {
		srv.Hub().BroadcastEvent("clipboard_url", detectedURL)
	})
	clipboardWatcher.Start(context.Background())

	// Wait briefly for server to bind
	time.Sleep(200 * time.Millisecond)

	// 7. Launch Desktop Window
	if !*noGui {
		uiURL := fmt.Sprintf("http://%s/index.html", addr)
		openWebView(uiURL, *minimized, func(w webview2.WebView) {
			activeWebView = w
		})
	} else {
		log.Println("IFD running in headless daemon mode. Press Ctrl+C to exit.")
		select {}
	}
}

func formatSpeed(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B/s", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB/s", float64(b)/float64(div), "KMGTPE"[exp])
}

func openWebView(url string, startMinimized bool, onReady func(w webview2.WebView)) {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "Nova Download Manager PRO",
			Width:  1100,
			Height: 720,
			IconId: 1,
			Center: true,
		},
	})
	if w == nil {
		log.Println("Edge WebView2 runtime not found, falling back to background daemon.")
		select {}
		return
	}
	defer w.Destroy()

	hwnd := uintptr(w.Window())
	platform.SetupFramelessWindow(hwnd)

	// Bind native window action for instant zero-latency window control
	_ = w.Bind("nativeWindowAction", func(action string) error {
		platform.PerformWindowAction(hwnd, action)
		return nil
	})

	// Bind native clipboard reader for instant zero-permission clipboard access
	_ = w.Bind("nativeReadClipboard", func() (string, error) {
		return platform.GetClipboardText(), nil
	})

	w.SetSize(1100, 720, webview2.HintNone)
	w.Navigate(url)

	if onReady != nil {
		onReady(w)
	}

	if startMinimized {
		platform.HideAppWindow()
	}

	w.Run()
}
