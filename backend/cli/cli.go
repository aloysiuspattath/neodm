package cli

import (
	"flag"
	"fmt"
	"math"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"ifd/backend/engine"
)

const Version = "1.1.8"

// RunCLI checks os.Args. If a CLI command or direct URL is requested,
// it executes the CLI action and returns true. If GUI should launch, it returns false.
func RunCLI(args []string) bool {
	if len(args) <= 1 {
		return false
	}

	firstArg := strings.ToLower(strings.TrimSpace(args[1]))

	// GUI flags should proceed to WebView2
	if firstArg == "-minimized" || firstArg == "--minimized" ||
		firstArg == "-nogui" || firstArg == "--nogui" ||
		firstArg == "-install" || firstArg == "--install" ||
		firstArg == "-uninstall" || firstArg == "--uninstall" {
		return false
	}

	// Direct URL shorthand: `neodm https://example.com/file.zip`
	if strings.HasPrefix(firstArg, "http://") || strings.HasPrefix(firstArg, "https://") || strings.HasPrefix(firstArg, "ftp://") {
		AttachConsoleIfNeeded()
		return runDownloadCmd(args[1:])
	}

	switch firstArg {
	case "download", "get":
		AttachConsoleIfNeeded()
		return runDownloadCmd(args[2:])
	case "list", "ls":
		AttachConsoleIfNeeded()
		return runListCmd()
	case "version", "-v", "--version":
		AttachConsoleIfNeeded()
		printVersion()
		return true
	case "help", "-h", "--help":
		AttachConsoleIfNeeded()
		printHelp()
		return true
	default:
		// If it looks like a flag for download, e.g. `neodm -o test.iso https://...`
		if strings.HasPrefix(firstArg, "-") {
			AttachConsoleIfNeeded()
			return runDownloadCmd(args[1:])
		}
		AttachConsoleIfNeeded()
		printHelp()
		return true
	}
}

func printVersion() {
	fmt.Printf("Neo Download Manager PRO (NeoDM) v%s\n", Version)
	fmt.Println("High-Performance Multi-Connection Turbo Download Engine")
	fmt.Println("Dual-Licensed: GNU General Public License v3.0 & Commercial OEM")
}

func printHelp() {
	fmt.Println("Neo Download Manager PRO (NeoDM) - Command Line Interface")
	fmt.Println("\nUsage:")
	fmt.Println("  neodm <url> [flags]                  Quick download a URL directly")
	fmt.Println("  neodm download <url> [flags]         Download with advanced options")
	fmt.Println("  neodm list                           Display all downloads and their status")
	fmt.Println("  neodm version                        Display version information")
	fmt.Println("  neodm help                           Display this help message")
	fmt.Println("\nDownload Flags:")
	fmt.Println("  -o, --out <filename>      Custom destination file name")
	fmt.Println("  -d, --dir <directory>     Destination download directory")
	fmt.Println("  -c, --conns <number>      Concurrent segmented connections (default: 16)")
	fmt.Println("  -s, --speed <limit>       Max speed limit (e.g. 5M, 500K, or bytes/sec)")
	fmt.Println("\nExamples:")
	fmt.Println("  neodm https://speed.cloudflare.com/__down?bytes=104857600 -o 100mb.bin")
	fmt.Println("  neodm download https://example.com/archive.tar.gz -d C:\\Downloads -c 32")
	fmt.Println("  neodm list")
}

func runListCmd() bool {
	store, err := engine.NewStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading storage: %v\n", err)
		return true
	}

	if len(store.Records) == 0 {
		fmt.Println("No downloads recorded in NeoDM.")
		return true
	}

	fmt.Printf("%-36s  %-24s  %-12s  %-10s  %s\n", "ID", "FILENAME", "SIZE", "STATUS", "PATH")
	fmt.Println(strings.Repeat("-", 100))

	for _, r := range store.Records {
		pct := 0
		if r.TotalSize > 0 {
			pct = int((float64(r.DownloadedBytes) / float64(r.TotalSize)) * 100)
		}
		sizeStr := formatBytes(r.TotalSize)
		statusStr := fmt.Sprintf("%s (%d%%)", r.Status, pct)

		fn := r.Filename
		if len(fn) > 24 {
			fn = fn[:21] + "..."
		}
		path := r.SavePath
		if len(path) > 30 {
			path = "..." + path[len(path)-27:]
		}

		fmt.Printf("%-36s  %-24s  %-12s  %-10s  %s\n", r.ID, fn, sizeStr, statusStr, path)
	}
	return true
}

func runDownloadCmd(args []string) bool {
	fs := flag.NewFlagSet("download", flag.ContinueOnError)
	var outName, saveDir, speedStr string
	var conns int

	fs.StringVar(&outName, "o", "", "Custom filename")
	fs.StringVar(&outName, "out", "", "Custom filename")
	fs.StringVar(&saveDir, "d", "", "Save directory")
	fs.StringVar(&saveDir, "dir", "", "Save directory")
	fs.IntVar(&conns, "c", 16, "Number of concurrent connections")
	fs.IntVar(&conns, "conns", 16, "Number of concurrent connections")
	fs.StringVar(&speedStr, "s", "", "Speed limit (e.g. 10M, 500K)")
	fs.StringVar(&speedStr, "speed", "", "Speed limit")

	var flagArgs []string
	var rawURL string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flagArgs = append(flagArgs, args[i+1])
				i++
			}
		} else if rawURL == "" {
			rawURL = arg
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		return true
	}

	if rawURL == "" {
		fmt.Fprintln(os.Stderr, "Error: No download URL provided.")
		fmt.Fprintln(os.Stderr, "Run 'neodm help' for usage instructions.")
		return true
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		fmt.Fprintf(os.Stderr, "Error: Invalid URL %q (only HTTP/HTTPS supported)\n", rawURL)
		return true
	}

	speedLimit := parseSpeed(speedStr)

	store, err := engine.NewStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing storage: %v\n", err)
		return true
	}

	eng := engine.NewEngine(store)
	if speedLimit > 0 {
		eng.SetGlobalSpeedLimit(speedLimit)
	}

	if saveDir == "" {
		saveDir = store.Settings.DefaultDownloadDir
		if saveDir == "" {
			saveDir, _ = os.Getwd()
		}
	}

	var mu sync.Mutex
	doneChan := make(chan struct{})
	var completedTask *engine.Task

	eng.SetCallbacks(func(t *engine.Task) {
		mu.Lock()
		defer mu.Unlock()
		renderProgressBar(t)
	}, func(t *engine.Task) {
		mu.Lock()
		completedTask = t
		mu.Unlock()
		select {
		case <-doneChan:
		default:
			close(doneChan)
		}
	})

	fmt.Println("🚀 Neo Download Manager PRO (Turbo Engine)")
	fmt.Printf("Connecting to: %s\n", rawURL)
	if outName != "" {
		fmt.Printf("Saving as:     %s\n", outName)
	}
	fmt.Printf("Destination:   %s\n", filepath.Clean(saveDir))
	fmt.Println(strings.Repeat("-", 70))

	task, err := eng.AddTask(rawURL, outName, saveDir, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start download: %v\n", err)
		return true
	}

	if conns > 0 {
		task.SetMaxConns(conns)
	}

	// Handle graceful cancellation on Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-doneChan:
		fmt.Println()
		if completedTask != nil && completedTask.Status == "Completed" {
			rec := completedTask.ToRecord()
			fmt.Printf("✓ Download complete: %s\n", rec.SavePath)
			fmt.Printf("  Size: %s | Final Speed: %s\n", formatBytes(rec.TotalSize), formatSpeed(rec.Speed))
		} else if completedTask != nil && completedTask.Status == "Error" {
			fmt.Fprintf(os.Stderr, "✗ Download failed: %s\n", completedTask.Error)
		}
	case <-sigChan:
		fmt.Println("\nDownload paused by user.")
		task.Pause()
	}

	return true
}

func renderProgressBar(t *engine.Task) {
	if t == nil {
		return
	}

	rec := t.ToRecord()
	pct := 0.0
	if rec.TotalSize > 0 {
		pct = (float64(rec.DownloadedBytes) / float64(rec.TotalSize)) * 100.0
		if pct > 100.0 {
			pct = 100.0
		}
	}

	barWidth := 24
	filled := int((pct / 100.0) * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)

	etaStr := "--:--"
	if rec.ETA > 0 {
		m := rec.ETA / 60
		s := rec.ETA % 60
		etaStr = fmt.Sprintf("%02d:%02d", m, s)
	}

	line := fmt.Sprintf("\r[%s] %5.1f%% | %s / %s | %s | ETA: %s  ",
		bar,
		pct,
		formatBytes(rec.DownloadedBytes),
		formatBytes(rec.TotalSize),
		formatSpeed(rec.Speed),
		etaStr,
	)

	fmt.Print(line)
}

func parseSpeed(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return 0
	}
	multiplier := int64(1)
	if strings.HasSuffix(s, "MB/S") || strings.HasSuffix(s, "M") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "MB/S"), "M")
	} else if strings.HasSuffix(s, "KB/S") || strings.HasSuffix(s, "K") {
		multiplier = 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "KB/S"), "K")
	}
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(val * float64(multiplier))
}

func formatBytes(b int64) string {
	if b <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := int(math.Floor(math.Log(float64(b)) / math.Log(1024)))
	if i >= len(units) {
		i = len(units) - 1
	}
	val := float64(b) / math.Pow(1024, float64(i))
	if i > 1 {
		return fmt.Sprintf("%.2f %s", val, units[i])
	}
	return fmt.Sprintf("%.1f %s", val, units[i])
}

func formatSpeed(bps int64) string {
	if bps <= 0 {
		return "0.0 KB/s"
	}
	if bps < 1024*1024 {
		return fmt.Sprintf("%.1f KB/s", float64(bps)/1024)
	}
	return fmt.Sprintf("%.2f MB/s", float64(bps)/(1024*1024))
}
