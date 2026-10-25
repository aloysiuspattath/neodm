package platform

import (
	"context"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32Clipboard            = syscall.NewLazyDLL("user32.dll")
	kernel32Clipboard          = syscall.NewLazyDLL("kernel32.dll")
	procOpenClipboard          = user32Clipboard.NewProc("OpenClipboard")
	procCloseClipboard         = user32Clipboard.NewProc("CloseClipboard")
	procGetClipboardData       = user32Clipboard.NewProc("GetClipboardData")
	procIsClipboardFormatAvail = user32Clipboard.NewProc("IsClipboardFormatAvailable")
	procGlobalLock             = kernel32Clipboard.NewProc("GlobalLock")
	procGlobalUnlock           = kernel32Clipboard.NewProc("GlobalUnlock")
)

const cfUnicodeText = 13 // CF_UNICODETEXT

// ClipboardWatcher monitors Windows clipboard for downloadable URLs.
type ClipboardWatcher struct {
	mu       sync.Mutex
	lastText string
	onURL    func(detectedURL string)
}

// NewClipboardWatcher creates a clipboard monitor.
func NewClipboardWatcher(onURL func(string)) *ClipboardWatcher {
	return &ClipboardWatcher{
		onURL: onURL,
	}
}

// Start begins the background monitoring loop.
func (cw *ClipboardWatcher) Start(ctx context.Context) {
	ticker := time.NewTicker(1500 * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				text := getClipboardText()
				if text == "" {
					continue
				}

				cw.mu.Lock()
				if text == cw.lastText {
					cw.mu.Unlock()
					continue
				}
				cw.lastText = text
				cw.mu.Unlock()

				// Validate if clipboard content is a downloadable URL
				cleanText := strings.TrimSpace(text)
				if (strings.HasPrefix(cleanText, "http://") || strings.HasPrefix(cleanText, "https://")) && !strings.Contains(cleanText, "\n") {
					if _, err := url.ParseRequestURI(cleanText); err == nil {
						if isDownloadableURL(cleanText) && cw.onURL != nil {
							cw.onURL(cleanText)
						}
					}
				}
			}
		}
	}()
}

// GetClipboardText retrieves current plain text from the OS clipboard.
func GetClipboardText() string {
	return getClipboardText()
}

func getClipboardTextNative() (string, bool) {
	var opened bool
	for i := 0; i < 5; i++ {
		r, _, _ := procOpenClipboard.Call(0)
		if r != 0 {
			opened = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !opened {
		return "", false
	}
	defer procCloseClipboard.Call()

	r, _, _ := procIsClipboardFormatAvail.Call(uintptr(cfUnicodeText))
	if r == 0 {
		return "", true // Opened successfully, but not text format (e.g. image or empty)
	}

	hMem, _, _ := procGetClipboardData.Call(uintptr(cfUnicodeText))
	if hMem == 0 {
		return "", true
	}

	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return "", true
	}
	defer procGlobalUnlock.Call(hMem)

	u16 := (*[1 << 22]uint16)(unsafe.Pointer(ptr))
	var length int
	for length = 0; length < (1<<22)-1; length++ {
		if u16[length] == 0 {
			break
		}
	}
	if length == 0 {
		return "", true
	}
	return syscall.UTF16ToString(u16[:length]), true
}

func getClipboardText() string {
	if text, ok := getClipboardTextNative(); ok {
		return strings.TrimSpace(text)
	}

	// Fallback to PowerShell if native Win32 clipboard access failed
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func isDownloadableURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	path := strings.ToLower(parsed.Path)
	commonExts := []string{
		".zip", ".rar", ".7z", ".tar", ".gz", ".iso",
		".exe", ".msi", ".apk", ".bin",
		".mp4", ".mkv", ".webm", ".avi",
		".mp3", ".flac", ".wav", ".m4a",
		".pdf", ".doc", ".docx", ".xls", ".xlsx",
	}
	for _, ext := range commonExts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	// Also check query param if path has no extension
	if strings.Contains(parsed.RawQuery, "download") || strings.Contains(parsed.RawQuery, "file=") || strings.Contains(parsed.RawQuery, "filename=") {
		return true
	}
	return false
}
