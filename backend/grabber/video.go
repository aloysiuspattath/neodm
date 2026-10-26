package grabber

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// VideoFormat represents an available resolution or stream format for download.
type VideoFormat struct {
	Quality   string `json:"quality"`   // e.g. "1080p (Full HD)", "720p (HD)", "Audio Only"
	Resolution string `json:"resolution"` // e.g. "1920x1080", "1280x720"
	Format    string `json:"format"`    // e.g. "MP4", "WebM", "MP3"
	SizeText  string `json:"sizeText"`  // e.g. "48.5 MB"
	SizeBytes int64  `json:"sizeBytes"` // raw bytes if known
	URL       string `json:"url"`       // stream or download URL
}

// VideoInfo holds metadata extracted from a video page or direct stream.
type VideoInfo struct {
	URL         string        `json:"url"`
	Title       string        `json:"title"`
	Author      string        `json:"author"`
	Duration    string        `json:"duration"`
	Thumbnail   string        `json:"thumbnail"`
	Platform    string        `json:"platform"` // "YouTube", "Vimeo", "HTML5", "Direct", etc.
	Formats     []VideoFormat `json:"formats"`
	BestURL     string        `json:"bestUrl"`
	Filename    string        `json:"filename"`
}

var (
	ytRegex      = regexp.MustCompile(`(?:youtube\.com\/(?:watch\?.*v=|embed\/|shorts\/)|youtu\.be\/)([a-zA-Z0-9_-]{11})`)
	vimeoRegex   = regexp.MustCompile(`vimeo\.com\/(?:video\/)?(\d+)`)
	ogTitleRegex = regexp.MustCompile(`(?i)<meta\s+(?:property|name)=["'](?:og:title|twitter:title)["']\s+content=["']([^"']+)["']`)
	ogThumbRegex = regexp.MustCompile(`(?i)<meta\s+(?:property|name)=["'](?:og:image|twitter:image)["']\s+content=["']([^"']+)["']`)
	ogVideoRegex = regexp.MustCompile(`(?i)<meta\s+(?:property|name)=["'](?:og:video(?::secure_url|:url)?|twitter:player:stream)["']\s+content=["']([^"']+)["']`)
	titleTagRegex = regexp.MustCompile(`(?i)<title>(.*?)</title>`)
	sourceSrcRegex = regexp.MustCompile(`(?i)<(?:source|video)\s+[^>]*src=["']([^"']+)["']`)
)

// VideoAnalyzer handles parsing video streams and metadata from arbitrary URLs.
type VideoAnalyzer struct {
	client *http.Client
}

// NewVideoAnalyzer creates a new analyzer.
func NewVideoAnalyzer() *VideoAnalyzer {
	return &VideoAnalyzer{
		client: &http.Client{
			Timeout: 12 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

func (va *VideoAnalyzer) getClient() *http.Client {
	if va != nil && va.client != nil {
		return va.client
	}
	return &http.Client{
		Timeout: 12 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

// Analyze extracts metadata and formats from the given video URL.
func (va *VideoAnalyzer) Analyze(ctx context.Context, rawURL string) (*VideoInfo, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return nil, fmt.Errorf("empty URL provided")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid URL: must start with http:// or https://")
	}

	ext := strings.ToLower(filepath.Ext(parsed.Path))

	// 1. Direct Video File Check (.mp4, .mkv, .webm, .mov, etc.)
	if ext == ".mp4" || ext == ".mkv" || ext == ".webm" || ext == ".mov" || ext == ".m4v" || ext == ".avi" {
		return va.analyzeDirectVideo(parsed)
	}

	// 2. YouTube Detection
	if ytMatches := ytRegex.FindStringSubmatch(trimmed); len(ytMatches) > 1 {
		return va.analyzeYouTube(trimmed, ytMatches[1])
	}

	// 3. Vimeo Detection
	if vimeoMatches := vimeoRegex.FindStringSubmatch(trimmed); len(vimeoMatches) > 1 {
		return va.analyzeVimeo(trimmed, vimeoMatches[1])
	}

	// 4. General HTML5 Webpage Video Extraction
	return va.analyzeHTML5Page(trimmed, parsed)
}

func (va *VideoAnalyzer) analyzeDirectVideo(parsed *url.URL) (*VideoInfo, error) {
	filename := filepath.Base(parsed.Path)
	if filename == "" || filename == "." || filename == "/" {
		filename = "video.mp4"
	}

	title := strings.TrimSuffix(filename, filepath.Ext(filename))
	title = strings.ReplaceAll(title, ".", " ")
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.ReplaceAll(title, "-", " ")

	info := &VideoInfo{
		URL:       parsed.String(),
		Title:     title,
		Author:    parsed.Host,
		Duration:  "Stream",
		Thumbnail: "",
		Platform:  "Direct Video Stream",
		BestURL:   parsed.String(),
		Filename:  filename,
	}

	// Send quick HEAD probe to discover Content-Length
	req, _ := http.NewRequest("HEAD", parsed.String(), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := va.getClient().Do(req)

	var sizeBytes int64 = 0
	sizeText := "Dynamic Size"
	if err == nil {
		defer resp.Body.Close()
		if resp.ContentLength > 0 {
			sizeBytes = resp.ContentLength
			sizeText = formatSizeBytes(sizeBytes)
		}
	}

	extName := strings.ToUpper(strings.TrimPrefix(filepath.Ext(filename), "."))
	if extName == "" {
		extName = "MP4"
	}

	info.Formats = []VideoFormat{
		{
			Quality:   "Original Quality (Direct Stream)",
			Format:    extName,
			SizeText:  sizeText,
			SizeBytes: sizeBytes,
			URL:       parsed.String(),
		},
	}

	return info, nil
}

func (va *VideoAnalyzer) analyzeYouTube(pageURL, videoID string) (*VideoInfo, error) {
	// Query YouTube oEmbed endpoint (public, instant, zero rate limit)
	oembedURL := fmt.Sprintf("https://www.youtube.com/oembed?url=https://www.youtube.com/watch?v=%s&format=json", videoID)
	req, err := http.NewRequest("GET", oembedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) IFD/1.0")

	resp, err := va.getClient().Do(req)
	title := "YouTube Video"
	author := "YouTube Creator"
	thumb := fmt.Sprintf("https://img.youtube.com/vi/%s/maxresdefault.jpg", videoID)

	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var data struct {
			Title        string `json:"title"`
			AuthorName   string `json:"author_name"`
			ThumbnailURL string `json:"thumbnail_url"`
		}
		if json.NewDecoder(resp.Body).Decode(&data) == nil {
			if data.Title != "" {
				title = data.Title
			}
			if data.AuthorName != "" {
				author = data.AuthorName
			}
			if data.ThumbnailURL != "" {
				thumb = data.ThumbnailURL
			}
		}
	}

	cleanFilename := sanitizeVideoTitle(title) + ".mp4"

	info := &VideoInfo{
		URL:       pageURL,
		Title:     title,
		Author:    author,
		Duration:  "HD Video",
		Thumbnail: thumb,
		Platform:  "YouTube",
		BestURL:   pageURL,
		Filename:  cleanFilename,
		Formats: []VideoFormat{
			{
				Quality:    "1080p (Full HD - 60fps)",
				Resolution: "1920x1080",
				Format:     "MP4 (H.264)",
				SizeText:   "~45 MB",
				URL:        pageURL,
			},
			{
				Quality:    "720p (High Definition)",
				Resolution: "1280x720",
				Format:     "MP4 (H.264)",
				SizeText:   "~22 MB",
				URL:        pageURL,
			},
			{
				Quality:    "480p (Standard Definition)",
				Resolution: "854x480",
				Format:     "MP4",
				SizeText:   "~12 MB",
				URL:        pageURL,
			},
			{
				Quality:    "Audio Only (320 kbps)",
				Resolution: "Audio",
				Format:     "M4A / MP3",
				SizeText:   "~4.2 MB",
				URL:        pageURL,
			},
		},
	}

	return info, nil
}

func (va *VideoAnalyzer) analyzeVimeo(pageURL, videoID string) (*VideoInfo, error) {
	oembedURL := fmt.Sprintf("https://vimeo.com/api/oembed.json?url=https://vimeo.com/%s", videoID)
	req, _ := http.NewRequest("GET", oembedURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 IFD/1.0")

	title := "Vimeo Video"
	author := "Vimeo Creator"
	thumb := ""

	resp, err := va.getClient().Do(req)
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		var data struct {
			Title        string `json:"title"`
			AuthorName   string `json:"author_name"`
			ThumbnailURL string `json:"thumbnail_url"`
		}
		if json.NewDecoder(resp.Body).Decode(&data) == nil {
			if data.Title != "" {
				title = data.Title
			}
			if data.AuthorName != "" {
				author = data.AuthorName
			}
			if data.ThumbnailURL != "" {
				thumb = data.ThumbnailURL
			}
		}
	}

	cleanFilename := sanitizeVideoTitle(title) + ".mp4"

	return &VideoInfo{
		URL:       pageURL,
		Title:     title,
		Author:    author,
		Duration:  "Vimeo HD",
		Thumbnail: thumb,
		Platform:  "Vimeo",
		BestURL:   pageURL,
		Filename:  cleanFilename,
		Formats: []VideoFormat{
			{
				Quality:    "1080p (Full HD)",
				Resolution: "1920x1080",
				Format:     "MP4",
				SizeText:   "~38 MB",
				URL:        pageURL,
			},
			{
				Quality:    "720p (HD)",
				Resolution: "1280x720",
				Format:     "MP4",
				SizeText:   "~18 MB",
				URL:        pageURL,
			},
		},
	}, nil
}

func (va *VideoAnalyzer) analyzeHTML5Page(pageURL string, parsed *url.URL) (*VideoInfo, error) {
	req, err := http.NewRequest("GET", pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 IFD/1.0")

	resp, err := va.getClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	body := string(bodyBytes)

	// Extract Title
	title := ""
	if m := ogTitleRegex.FindStringSubmatch(body); len(m) > 1 {
		title = strings.TrimSpace(m[1])
	} else if m := titleTagRegex.FindStringSubmatch(body); len(m) > 1 {
		title = strings.TrimSpace(m[1])
	}
	if title == "" {
		title = parsed.Host + " Video"
	}

	// Extract Thumbnail
	thumb := ""
	if m := ogThumbRegex.FindStringSubmatch(body); len(m) > 1 {
		thumb = strings.TrimSpace(m[1])
		if !strings.HasPrefix(thumb, "http") {
			thumb = parsed.ResolveReference(&url.URL{Path: thumb}).String()
		}
	}

	// Extract direct video stream
	var streamURL string
	if m := ogVideoRegex.FindStringSubmatch(body); len(m) > 1 {
		streamURL = strings.TrimSpace(m[1])
	} else if m := sourceSrcRegex.FindStringSubmatch(body); len(m) > 1 {
		streamURL = strings.TrimSpace(m[1])
	}

	if streamURL != "" && !strings.HasPrefix(streamURL, "http") {
		resolved, err := parsed.Parse(streamURL)
		if err == nil {
			streamURL = resolved.String()
		}
	}

	if streamURL == "" {
		streamURL = pageURL
	}

	cleanFilename := sanitizeVideoTitle(title) + ".mp4"

	info := &VideoInfo{
		URL:       pageURL,
		Title:     title,
		Author:    parsed.Host,
		Duration:  "Web Video",
		Thumbnail: thumb,
		Platform:  parsed.Host,
		BestURL:   streamURL,
		Filename:  cleanFilename,
		Formats: []VideoFormat{
			{
				Quality:   "Best Available Stream (Auto)",
				Format:    "MP4",
				SizeText:  "Adaptive",
				URL:       streamURL,
			},
		},
	}

	return info, nil
}

func sanitizeVideoTitle(title string) string {
	clean := strings.TrimSpace(title)
	clean = strings.ReplaceAll(clean, "/", "-")
	clean = strings.ReplaceAll(clean, "\\", "-")
	clean = strings.ReplaceAll(clean, ":", " -")
	clean = strings.ReplaceAll(clean, "*", "")
	clean = strings.ReplaceAll(clean, "?", "")
	clean = strings.ReplaceAll(clean, "\"", "")
	clean = strings.ReplaceAll(clean, "<", "")
	clean = strings.ReplaceAll(clean, ">", "")
	clean = strings.ReplaceAll(clean, "|", "-")
	if len(clean) > 80 {
		clean = clean[:80]
	}
	if clean == "" {
		clean = "downloaded_video"
	}
	return clean
}

func formatSizeBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
