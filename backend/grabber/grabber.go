package grabber

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GrabbedLink represents an extracted resource ready for queueing.
type GrabbedLink struct {
	URL       string `json:"url"`
	Filename  string `json:"filename"`
	Extension string `json:"extension"`
	Category  string `json:"category"`
	Size      int64  `json:"size"`
}

// GrabberOptions parameters for crawling and filtering links.
type GrabberOptions struct {
	URL             string   `json:"url"`
	Depth           int      `json:"depth"` // 1 = page only, 2 = 1 level deep links
	RegexFilter     string   `json:"regexFilter"`
	Extensions      []string `json:"extensions"`      // e.g. ["mp4", "mkv"] or ["pdf"]
	Categories      []string `json:"categories"`      // e.g. ["Videos", "Documents"]
	ProbeFileSizes  bool     `json:"probeFileSizes"`  // Send HEAD to retrieve size
	CustomHeaders   map[string]string `json:"headers"`
}

// SiteGrabber crawls websites and extracts matching links.
type SiteGrabber struct {
	client *http.Client
}

// NewSiteGrabber creates a new grabber instance.
func NewSiteGrabber() *SiteGrabber {
	return &SiteGrabber{
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// Crawl crawls the target URL and returns filtered links.
func (g *SiteGrabber) Crawl(opts GrabberOptions) ([]*GrabbedLink, error) {
	parsedRoot, err := url.Parse(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	visited := make(map[string]bool)
	foundURLs := make(map[string]bool)
	var mu sync.Mutex

	var re *regexp.Regexp
	if opts.RegexFilter != "" {
		compiled, err := regexp.Compile(opts.RegexFilter)
		if err == nil {
			re = compiled
		}
	}

	extMap := make(map[string]bool)
	for _, ext := range opts.Extensions {
		clean := strings.ToLower(strings.TrimPrefix(ext, "."))
		if clean != "" {
			extMap[clean] = true
		}
	}

	catMap := make(map[string]bool)
	for _, cat := range opts.Categories {
		catMap[strings.ToLower(cat)] = true
	}

	var crawlPage func(targetURL string, currentDepth int)
	crawlPage = func(targetURL string, currentDepth int) {
		mu.Lock()
		if visited[targetURL] || currentDepth > opts.Depth {
			mu.Unlock()
			return
		}
		visited[targetURL] = true
		mu.Unlock()

		req, err := http.NewRequest("GET", targetURL, nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 NeoDM/1.1")
		for k, v := range opts.CustomHeaders {
			req.Header.Set(k, v)
		}

		resp, err := g.client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			return
		}
		defer resp.Body.Close()

		// Read up to 5MB of HTML
		bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
		if err != nil {
			return
		}
		bodyStr := string(bodyBytes)

		base, _ := url.Parse(targetURL)
		extractedLinks := extractLinksFromHTML(bodyStr)

		var subPages []string

		for _, rawLink := range extractedLinks {
			resolved := base.ResolveReference(rawLink)
			if resolved.Scheme != "http" && resolved.Scheme != "https" {
				continue
			}

			fullURL := resolved.String()

			// Check if candidate link is a subpage for deeper crawling (same domain)
			if currentDepth < opts.Depth && resolved.Host == parsedRoot.Host {
				subPages = append(subPages, fullURL)
			}

			// Apply regex filter if provided
			if re != nil && !re.MatchString(fullURL) {
				continue
			}

			// Extract extension
			ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(resolved.Path), "."))

			// Apply extension filter
			if len(extMap) > 0 && !extMap[ext] {
				continue
			}

			mu.Lock()
			foundURLs[fullURL] = true
			mu.Unlock()
		}

		// Recurse into subpages if depth allows
		for _, sp := range subPages {
			crawlPage(sp, currentDepth+1)
		}
	}

	crawlPage(opts.URL, 1)

	var results []*GrabbedLink
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10) // Concurrency throttle for HEAD requests

	for u := range foundURLs {
		parsed, _ := url.Parse(u)
		filename := filepath.Base(parsed.Path)
		if filename == "" || filename == "." || filename == "/" {
			filename = "download"
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
		cat := detectCategory(ext)

		if len(catMap) > 0 && !catMap[strings.ToLower(cat)] {
			continue
		}

		link := &GrabbedLink{
			URL:       u,
			Filename:  filename,
			Extension: ext,
			Category:  cat,
			Size:      0,
		}

		if opts.ProbeFileSizes {
			wg.Add(1)
			go func(item *GrabbedLink) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				req, err := http.NewRequest("HEAD", item.URL, nil)
				if err == nil {
					req.Header.Set("User-Agent", "Mozilla/5.0 NeoDM/1.1")
					resp, err := g.client.Do(req)
					if err == nil {
						item.Size = resp.ContentLength
						resp.Body.Close()
					}
				}
			}(link)
		}

		results = append(results, link)
	}

	if opts.ProbeFileSizes {
		wg.Wait()
	}

	return results, nil
}

// ExpandBatchPattern expands templates like "http://example.com/file_[01-10].zip"
func ExpandBatchPattern(template string) []string {
	re := regexp.MustCompile(`\[(\d+)-(\d+)\]`)
	matches := re.FindStringSubmatch(template)
	if len(matches) != 3 {
		return []string{template}
	}

	startStr := matches[1]
	endStr := matches[2]
	start, err1 := strconv.Atoi(startStr)
	end, err2 := strconv.Atoi(endStr)
	if err1 != nil || err2 != nil || start > end {
		return []string{template}
	}

	// Limit expansion to 1000 items
	if end-start > 1000 {
		end = start + 1000
	}

	padZeroes := len(startStr) == len(endStr) && strings.HasPrefix(startStr, "0")
	format := "%d"
	if padZeroes {
		format = fmt.Sprintf("%%0%dd", len(startStr))
	}

	var results []string
	for i := start; i <= end; i++ {
		numStr := fmt.Sprintf(format, i)
		expanded := strings.Replace(template, matches[0], numStr, 1)
		results = append(results, expanded)
	}

	return results
}

// extractLinksFromHTML finds href, src, and video/audio sources
func extractLinksFromHTML(html string) []*url.URL {
	var links []*url.URL
	seen := make(map[string]bool)

	// Regex pattern for attributes like href="...", href='...', or unquoted href=/path
	attrRe := regexp.MustCompile(`(?i)(?:href|src|data-src|data-url)\s*=\s*(?:"([^">#\s]+)"|'([^'>#\s]+)'|([^\s>]+))`)
	matches := attrRe.FindAllStringSubmatch(html, -1)

	for _, m := range matches {
		raw := ""
		if len(m) > 1 && m[1] != "" {
			raw = m[1]
		} else if len(m) > 2 && m[2] != "" {
			raw = m[2]
		} else if len(m) > 3 && m[3] != "" {
			raw = m[3]
		}

		raw = strings.Trim(raw, `"'`)
		raw = strings.TrimSpace(raw)
		if raw != "" && !seen[raw] && !strings.HasPrefix(raw, "data:") && !strings.HasPrefix(raw, "javascript:") {
			seen[raw] = true
			if parsed, err := url.Parse(raw); err == nil {
				links = append(links, parsed)
			}
		}
	}

	return links
}

func detectCategory(ext string) string {
	switch ext {
	case "zip", "rar", "7z", "tar", "gz", "bz2", "xz", "iso", "dmg":
		return "Compressed"
	case "pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "txt":
		return "Documents"
	case "mp4", "mkv", "avi", "mov", "webm", "flv", "m4v", "ts":
		return "Videos"
	case "mp3", "flac", "wav", "aac", "ogg", "m4a":
		return "Music"
	case "exe", "msi", "bin", "apk":
		return "Programs"
	default:
		return "Other"
	}
}
