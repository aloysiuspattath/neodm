package platform

import (
	"context"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

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

func getClipboardText() string {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard")
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
	return false
}
