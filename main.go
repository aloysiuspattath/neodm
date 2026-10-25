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

	minimized := flag.Bool("minimized", false, "Start minimized in system tray")
	port := flag.Int("port", 19765, "Local HTTP/WebSocket server port")
	noGui := flag.Bool("nogui", false, "Run in headless daemon mode")
	installFlag := flag.Bool("install", false, "Install IFD, create desktop/start shortcuts, and register uninstaller")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall IFD and remove shortcuts")
	flag.Parse()

	if *installFlag {
		if err := platform.InstallApp(); err != nil {
			log.Fatalf("Installation failed: %v", err)
		}
		fmt.Println("IFD installed successfully! Shortcuts created.")
		return
	}

	if *uninstallFlag {
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
		platform.ShowNotification("NeoDM - Download Completed", fmt.Sprintf("%s finished downloading.", t.Filename))
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
		// If another instance is running, open UI or exit
		if !*noGui {
			openWebView(fmt.Sprintf("http://%s/index.html", addr), *minimized)
		}
		return
	}

	go func() {
		log.Printf("NeoDM Engine listening on http://%s", addr)
		if err := srv.StartWithListener(listener); err != nil {
			log.Printf("Server stopped: %v", err)
		}
	}()

	// 5. Start clipboard monitoring
	clipboardWatcher := platform.NewClipboardWatcher(func(detectedURL string) {
		srv.Hub().BroadcastEvent("clipboard_url", detectedURL)
	})
	clipboardWatcher.Start(context.Background())

	// Wait briefly for server to bind
	time.Sleep(200 * time.Millisecond)

	// 6. Launch Desktop Window
	if !*noGui {
		uiURL := fmt.Sprintf("http://%s/index.html", addr)
		openWebView(uiURL, *minimized)
	} else {
		log.Println("IFD running in headless daemon mode. Press Ctrl+C to exit.")
		select {}
	}
}

func openWebView(url string, startMinimized bool) {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "Neo Download Manager PRO",
			Width:  1100,
			Height: 720,
			IconId: 0,
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

	w.SetSize(1100, 720, webview2.HintNone)
	w.Navigate(url)

	w.Run()
}
