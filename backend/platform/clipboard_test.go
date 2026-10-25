package platform

import (
	"testing"
)

func TestGetClipboardText(t *testing.T) {
	// Must execute without panic
	text := GetClipboardText()
	t.Logf("Current clipboard text: %q", text)
}

func TestIsDownloadableURL(t *testing.T) {
	valid := []string{
		"https://example.com/archive.zip",
		"http://cdn.net/installer.exe",
		"https://files.org/movie.mp4",
		"https://speed.cloudflare.com/files?download=true",
		"https://example.com/api?file=package.tar.gz",
	}
	for _, u := range valid {
		if !isDownloadableURL(u) {
			t.Errorf("Expected %q to be downloadable", u)
		}
	}

	invalid := []string{
		"https://google.com",
		"https://example.com/about",
		"not-a-url",
	}
	for _, u := range invalid {
		if isDownloadableURL(u) {
			t.Errorf("Expected %q to NOT be downloadable", u)
		}
	}
}
