package grabber

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnalyzeDirectVideo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "10485760") // 10 MB
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	analyzer := NewVideoAnalyzer()
	info, err := analyzer.Analyze(context.Background(), server.URL+"/sample_clip.mp4")
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if info.Platform != "Direct Video Stream" {
		t.Errorf("Expected platform Direct Video Stream, got %s", info.Platform)
	}

	if len(info.Formats) == 0 {
		t.Fatalf("Expected at least 1 format, got 0")
	}

	if info.Formats[0].SizeBytes != 10485760 {
		t.Errorf("Expected 10MB sizeBytes, got %d", info.Formats[0].SizeBytes)
	}
}

func TestAnalyzeHTML5Page(t *testing.T) {
	htmlContent := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>Awesome Animation Movie</title>
		<meta property="og:title" content="Awesome Animation Movie" />
		<meta property="og:image" content="https://example.com/poster.jpg" />
		<meta property="og:video" content="https://example.com/stream.mp4" />
	</head>
	<body>
		<video src="https://example.com/stream.mp4"></video>
	</body>
	</html>
	`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(htmlContent))
	}))
	defer server.Close()

	analyzer := NewVideoAnalyzer()
	info, err := analyzer.Analyze(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Analyze HTML5 page failed: %v", err)
	}

	if info.Title != "Awesome Animation Movie" {
		t.Errorf("Expected title 'Awesome Animation Movie', got '%s'", info.Title)
	}

	if info.Thumbnail != "https://example.com/poster.jpg" {
		t.Errorf("Expected thumbnail poster.jpg, got '%s'", info.Thumbnail)
	}

	if info.BestURL != "https://example.com/stream.mp4" {
		t.Errorf("Expected stream URL https://example.com/stream.mp4, got '%s'", info.BestURL)
	}
}

func TestSanitizeVideoTitle(t *testing.T) {
	raw := "Interstellar: 2014 / Official <Trailer> ? [1080p] | *HD*"
	clean := sanitizeVideoTitle(raw)
	expected := "Interstellar - 2014 - Official Trailer  [1080p] - HD"
	if clean != expected {
		t.Errorf("Expected %q, got %q", expected, clean)
	}
}

func TestAnalyzeYouTube(t *testing.T) {
	analyzer := NewVideoAnalyzer()
	info, err := analyzer.Analyze(context.Background(), "https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if err != nil {
		t.Fatalf("Analyze YouTube failed: %v", err)
	}
	if info.Platform != "YouTube" {
		t.Errorf("Expected platform YouTube, got %s", info.Platform)
	}
	if len(info.Formats) == 0 {
		t.Errorf("Expected formats for YouTube video, got 0")
	}
	if info.Filename == "" || info.Filename == ".mp4" || info.Filename == "-.mp4" {
		t.Errorf("Invalid filename generated: %q", info.Filename)
	}
}
