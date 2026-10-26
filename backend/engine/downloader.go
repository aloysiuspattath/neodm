package engine

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Task represents an active or queued download job.
type Task struct {
	mu            sync.RWMutex
	ID            string
	URL           string
	Filename      string
	SavePath      string
	FinalPath     string
	TotalSize     int64
	Status        string // Queued, Connecting, Downloading, Paused, Completed, Error
	Category      string
	QueueID       string
	QueueIndex    int
	Speed         int64
	ETA           int64 // seconds
	SupportsRange bool
	Headers       map[string]string
	MaxConns      int
	SpeedLimit    int64 // per-task speed limit in bytes/sec, 0 = unlimited
	CreatedAt     time.Time
	CompletedAt   *time.Time
	Error         string

	ctx           context.Context
	cancel        context.CancelFunc
	storage       *Storage
	segmentMgr    *SegmentManager
	engine        *Engine
	client        *http.Client
	localLimiter  *RateLimiter
	prevBytes     int64
	prevTime      time.Time
}

// Engine manages the collection of download tasks, queueing, and bandwidth allocation.
type Engine struct {
	mu                sync.RWMutex
	tasks             map[string]*Task
	store             *Store
	limiter           *RateLimiter
	client            *http.Client
	QueueMgr          *QueueManager
	updateCallbacks   []func(t *Task)
	completeCallbacks []func(t *Task)
}

// NewEngine creates a new Download Engine instance.
func NewEngine(store *Store) *Engine {
	// Custom HTTP client with optimized transport for high-throughput concurrency
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: false},
		ForceAttemptHTTP2:     true,
	}

	eng := &Engine{
		tasks:   make(map[string]*Task),
		store:   store,
		limiter: NewRateLimiter(store.Settings.GlobalSpeedLimit),
		client: &http.Client{
			Transport: transport,
			Timeout:   0, // No global timeout for long downloads
		},
	}
	eng.QueueMgr = NewQueueManager(eng, store)

	// Restore tasks from store
	for _, rec := range store.Records {
		// Reset any stale active statuses from a previous process termination
		if rec.Status == "Downloading" || rec.Status == "Connecting" {
			rec.Status = "Paused"
		}
		task := eng.taskFromRecord(rec)
		eng.tasks[task.ID] = task
		if eng.QueueMgr != nil {
			eng.QueueMgr.AssignTask(task.ID, task.QueueID)
		}
	}

	return eng
}

// SetCallbacks registers callbacks for UI updates and completion alerts.
func (e *Engine) SetCallbacks(onUpdate func(t *Task), onComplete func(t *Task)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if onUpdate != nil {
		e.updateCallbacks = append(e.updateCallbacks, onUpdate)
	}
	if onComplete != nil {
		e.completeCallbacks = append(e.completeCallbacks, onComplete)
	}
}

// OnTaskUpdate registers an update listener callback.
func (e *Engine) OnTaskUpdate(fn func(t *Task)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if fn != nil {
		e.updateCallbacks = append(e.updateCallbacks, fn)
	}
}

// OnTaskComplete registers a completion listener callback.
func (e *Engine) OnTaskComplete(fn func(t *Task)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if fn != nil {
		e.completeCallbacks = append(e.completeCallbacks, fn)
	}
}

// SetGlobalSpeedLimit updates bandwidth throttle.
func (e *Engine) SetGlobalSpeedLimit(bytesPerSec int64) {
	e.limiter.SetLimit(bytesPerSec)
	e.store.Settings.GlobalSpeedLimit = bytesPerSec
	_ = e.store.SaveSettings()
}

func (e *Engine) taskFromRecord(rec *DownloadRecord) *Task {
	t := &Task{
		ID:            rec.ID,
		URL:           rec.URL,
		Filename:      rec.Filename,
		SavePath:      rec.SavePath,
		FinalPath:     rec.FinalPath,
		TotalSize:     rec.TotalSize,
		Status:        rec.Status,
		Category:      rec.Category,
		QueueID:       rec.QueueID,
		QueueIndex:    rec.QueueIndex,
		SupportsRange: rec.SupportsRange,
		Headers:       rec.Headers,
		MaxConns:      rec.MaxConns,
		SpeedLimit:    rec.SpeedLimit,
		CreatedAt:     rec.CreatedAt,
		CompletedAt:   rec.CompletedAt,
		Error:         rec.Error,
		engine:        e,
		client:        e.client,
		localLimiter:  NewRateLimiter(rec.SpeedLimit),
	}

	if t.QueueID == "" {
		t.QueueID = "main_queue"
	}

	if t.MaxConns <= 0 {
		t.MaxConns = e.store.Settings.MaxConnectionsPerDownload
	}

	if len(rec.Segments) > 0 {
		sm := &SegmentManager{
			totalSize: rec.TotalSize,
			segments:  rec.Segments,
			nextID:    len(rec.Segments),
		}
		t.segmentMgr = sm
	}

	return t
}

// DetectCategory categorizes a file based on its extension.
func DetectCategory(filename string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filename), "."))
	switch ext {
	case "zip", "rar", "7z", "tar", "gz", "bz2", "xz", "iso", "dmg", "tgz":
		return "Compressed"
	case "pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "txt", "epub", "csv":
		return "Documents"
	case "mp4", "mkv", "avi", "mov", "webm", "flv", "wmv", "m4v", "ts":
		return "Videos"
	case "mp3", "flac", "wav", "aac", "ogg", "m4a", "wma", "opus":
		return "Music"
	case "jpg", "jpeg", "png", "webp", "gif", "svg", "bmp", "ico", "tiff", "psd":
		return "Pictures"
	case "exe", "msi", "bin", "apk", "appimage", "deb", "rpm":
		return "Programs"
	default:
		return "Other"
	}
}

// SanitizeFilename strips directory traversal sequences, unsafe characters, and Windows reserved names.
func SanitizeFilename(filename string) string {
	filename = filepath.Base(filename)
	// Replace illegal Windows characters
	unsafeChars := regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
	filename = unsafeChars.ReplaceAllString(filename, "_")
	filename = strings.Trim(filename, ". ")

	// Guard against Windows reserved names (CON, PRN, AUX, NUL, COM1-9, LPT1-9)
	upper := strings.ToUpper(filename)
	reserved := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}
	for _, res := range reserved {
		if upper == res || strings.HasPrefix(upper, res+".") {
			filename = "_" + filename
			break
		}
	}

	if len(filename) > 200 {
		ext := filepath.Ext(filename)
		if len(ext) > 20 {
			ext = ""
		}
		baseLen := 200 - len(ext)
		if baseLen < 1 {
			baseLen = 1
		}
		filename = filename[:baseLen] + ext
	}

	if filename == "" || filename == "." {
		filename = "download"
	}
	return filename
}

// AddTask creates a new download task in the default queue.
func (e *Engine) AddTask(rawURL string, customFilename string, customDir string, headers map[string]string) (*Task, error) {
	return e.AddTaskWithQueue(rawURL, customFilename, customDir, headers, "")
}

// AddTaskWithQueue creates a new download task assigned to a specific queue.
func (e *Engine) AddTaskWithQueue(rawURL string, customFilename string, customDir string, headers map[string]string, queueID string) (*Task, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme %q, only http and https are allowed", parsedURL.Scheme)
	}

	filename := customFilename
	if filename == "" {
		filename = filepath.Base(parsedURL.Path)
	}
	filename = SanitizeFilename(filename)

	category := DetectCategory(filename)
	targetDir := customDir
	if targetDir == "" {
		if folder, exists := e.store.Settings.CategoryFolders[category]; exists && folder != "" {
			targetDir = folder
		} else {
			targetDir = e.store.Settings.DefaultDownloadDir
		}
	}

	// Ensure unique filename and save path
	ext := filepath.Ext(filename)
	baseWithoutExt := filename[:len(filename)-len(ext)]
	targetFile := filepath.Join(targetDir, filename)
	counter := 1

	e.mu.Lock()
	for {
		collision := false
		if _, err := os.Stat(targetFile); err == nil {
			collision = true
		}
		for _, existing := range e.tasks {
			if existing.SavePath == targetFile && existing.Status != "Completed" {
				collision = true
				break
			}
		}
		if !collision {
			break
		}
		filename = fmt.Sprintf("%s (%d)%s", baseWithoutExt, counter, ext)
		targetFile = filepath.Join(targetDir, filename)
		counter++
	}

	if queueID == "" {
		queueID = e.store.Settings.DefaultQueueID
	}
	if queueID == "" {
		queueID = "main_queue"
	}

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	task := &Task{
		ID:           id,
		URL:          rawURL,
		Filename:     filename,
		SavePath:     targetFile,
		Status:       "Queued",
		Category:     category,
		QueueID:      queueID,
		Headers:      headers,
		MaxConns:     e.store.Settings.MaxConnectionsPerDownload,
		SpeedLimit:   0,
		CreatedAt:    time.Now(),
		engine:       e,
		client:       e.client,
		localLimiter: NewRateLimiter(0),
	}

	e.tasks[id] = task
	e.mu.Unlock()

	if e.QueueMgr != nil {
		e.QueueMgr.AssignTask(id, queueID)
	}

	e.saveTask(task)
	e.checkQueue()

	// Ensure task starts immediately if queued without getting blocked
	if task.Status == "Queued" {
		go task.Start()
	}

	return task, nil
}

// checkQueue manages concurrency limits across global settings and per-queue scheduling.
func (e *Engine) checkQueue() {
	var queuesCopy []*QueueRecord
	if e.QueueMgr != nil {
		queuesCopy = e.QueueMgr.GetAllQueues()
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	activeCount := 0
	activePerQueue := make(map[string]int)

	for _, t := range e.tasks {
		if t.Status == "Downloading" || t.Status == "Connecting" {
			activeCount++
			qid := t.QueueID
			if qid == "" {
				qid = "main_queue"
			}
			activePerQueue[qid]++
		}
	}

	limit := e.store.Settings.MaxConcurrentDownloads
	if limit <= 0 {
		limit = 4
	}

	// 1. If QueueManager has registered queues, honor queue ordering and concurrency limits
	if len(queuesCopy) > 0 {
		for _, q := range queuesCopy {
			if !q.Running {
				continue
			}
			simLimit := q.SimultaneousDownloads
			if simLimit <= 0 {
				simLimit = 4
			}

			for _, taskID := range q.Order {
				if activeCount >= limit {
					return
				}
				if activePerQueue[q.ID] >= simLimit {
					break
				}
				t, exists := e.tasks[taskID]
				if exists && t.Status == "Queued" {
					activeCount++
					activePerQueue[q.ID]++
					go t.Start()
				}
			}
		}
	}

	// 2. Fallback for tasks not in queues or standalone
	if activeCount < limit {
		for _, t := range e.tasks {
			if activeCount >= limit {
				break
			}
			if t.Status == "Queued" {
				activeCount++
				go t.Start()
			}
		}
	}
}

// Start initiates the download process.
func (t *Task) Start() {
	t.mu.Lock()
	if t.Status != "Queued" {
		t.mu.Unlock()
		return
	}
	if t.engine.GetTask(t.ID) == nil {
		t.mu.Unlock()
		return
	}
	t.Status = "Connecting"
	t.Error = ""
	t.ctx, t.cancel = context.WithCancel(context.Background())
	t.prevBytes = 0
	t.prevTime = time.Now()
	t.mu.Unlock()

	t.engine.updateTask(t)

	// Step 1: Probe server capabilities
	if err := t.probeServer(); err != nil {
		if t.ctx.Err() != nil {
			return // Paused during probe
		}
		t.fail(err)
		return
	}
	if t.ctx.Err() != nil {
		return
	}

	// Step 2: Initialize storage with configurable temporary scratch directory
	var tempDir string
	if t.engine != nil && t.engine.store != nil && t.engine.store.Settings != nil && !t.engine.store.Settings.UseTargetForTemp {
		tempDir = t.engine.store.Settings.TemporaryDirectory
	}
	storage, err := NewStorage(t.SavePath, t.TotalSize, t.ID, tempDir)
	if err != nil {
		t.fail(err)
		return
	}
	t.storage = storage

	// Step 3: Initialize segment manager
	if t.segmentMgr == nil {
		conns := t.MaxConns
		if !t.SupportsRange {
			conns = 1
		}
		t.segmentMgr = NewSegmentManager(t.TotalSize, conns)
	} else if !t.SupportsRange {
		// Non-resumable download restarting: reset single segment offset to 0
		segments := t.segmentMgr.Segments()
		if len(segments) > 0 {
			atomic.StoreInt64(&segments[0].CurrentOffset, 0)
			segments[0].Done = false
		}
	}

	t.mu.Lock()
	t.Status = "Downloading"
	t.mu.Unlock()
	t.engine.updateTask(t)

	// Step 4: Launch worker routines
	var wg sync.WaitGroup
	segments := t.segmentMgr.Segments()

	for _, seg := range segments {
		if !seg.Done {
			wg.Add(1)
			go func(s *Segment) {
				defer wg.Done()
				t.runWorker(s, &wg)
			}(seg)
		}
	}

	// Step 5: Start progress monitor ticker
	ticker := time.NewTicker(500 * time.Millisecond)
	go func() {
		for {
			select {
			case <-t.ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				t.updateProgress()
			}
		}
	}()

	// Wait for all workers to finish
	wg.Wait()
	ticker.Stop()

	// Check completion status
	if t.ctx.Err() != nil {
		// Paused or cancelled
		return
	}

	if t.segmentMgr.IsComplete() {
		finalPath, err := t.storage.Finalize()
		if err != nil {
			t.fail(err)
			return
		}

		t.mu.Lock()
		t.FinalPath = finalPath
		t.Status = "Completed"
		now := time.Now()
		t.CompletedAt = &now
		t.Speed = 0
		t.ETA = 0
		t.mu.Unlock()

		t.engine.updateTask(t)
		t.engine.mu.RLock()
		completeCbs := make([]func(t *Task), len(t.engine.completeCallbacks))
		copy(completeCbs, t.engine.completeCallbacks)
		t.engine.mu.RUnlock()
		for _, cb := range completeCbs {
			cb(t)
		}
		if t.engine.QueueMgr != nil {
			t.engine.QueueMgr.CheckQueueProgression(t)
		} else {
			t.engine.checkQueue()
		}
	}
}

// probeServer checks file size, range capability, and content disposition.
func (t *Task) probeServer() error {
	probeCtx, cancel := context.WithTimeout(t.ctx, 15*time.Second)
	defer cancel()

	// 1. Try HEAD request first (standard HTTP probe)
	headReq, err := http.NewRequestWithContext(probeCtx, "HEAD", t.URL, nil)
	if err == nil {
		for k, v := range t.Headers {
			headReq.Header.Set(k, v)
		}
		if headReq.Header.Get("User-Agent") == "" {
			headReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 IFD/1.0")
		}

		headResp, err := t.client.Do(headReq)
		if err == nil {
			defer headResp.Body.Close()
			if headResp.StatusCode == http.StatusOK || headResp.StatusCode == http.StatusPartialContent {
				t.TotalSize = headResp.ContentLength
				acceptRanges := headResp.Header.Get("Accept-Ranges")
				if strings.EqualFold(acceptRanges, "bytes") {
					t.SupportsRange = true
				}
				t.extractFilename(headResp.Header)
				if t.TotalSize > 0 && t.SupportsRange {
					return nil
				}
			}
		}
	}

	// 2. Fallback to GET probe with Range: bytes=0-0 to test range support and size accurately
	req, err := http.NewRequestWithContext(probeCtx, "GET", t.URL, nil)
	if err == nil {
		for k, v := range t.Headers {
			req.Header.Set(k, v)
		}
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 IFD/1.0")
		}
		req.Header.Set("Range", "bytes=0-0")

		resp, err := t.client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusPartialContent {
				t.SupportsRange = true
				contentRange := resp.Header.Get("Content-Range")
				if contentRange != "" {
					var start, end, total int64
					if _, err := fmt.Sscanf(contentRange, "bytes %d-%d/%d", &start, &end, &total); err == nil {
						t.TotalSize = total
					}
				}
				t.extractFilename(resp.Header)
				return nil
			} else if resp.StatusCode == http.StatusOK {
				// Server ignored Range header and returned full content
				t.SupportsRange = strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes")
				if resp.ContentLength > 0 {
					t.TotalSize = resp.ContentLength
				}
				t.extractFilename(resp.Header)
				return nil
			}
		}
	}

	// 3. Fallback: plain GET request without Range header (handles servers returning 400/416 to Range)
	plainReq, err := http.NewRequestWithContext(probeCtx, "GET", t.URL, nil)
	if err != nil {
		return err
	}
	for k, v := range t.Headers {
		plainReq.Header.Set(k, v)
	}
	if plainReq.Header.Get("User-Agent") == "" {
		plainReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 IFD/1.0")
	}
	plainResp, err := t.client.Do(plainReq)
	if err != nil {
		return err
	}
	defer plainResp.Body.Close()

	if plainResp.StatusCode < 200 || plainResp.StatusCode >= 300 {
		return fmt.Errorf("server responded with status %d %s", plainResp.StatusCode, plainResp.Status)
	}

	t.SupportsRange = strings.EqualFold(plainResp.Header.Get("Accept-Ranges"), "bytes")
	if plainResp.ContentLength > 0 {
		t.TotalSize = plainResp.ContentLength
	}
	t.extractFilename(plainResp.Header)
	return nil
}

func (t *Task) extractFilename(h http.Header) {
	cd := h.Get("Content-Disposition")
	if cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if fn, ok := params["filename*"]; ok {
				parts := strings.SplitN(fn, "''", 2)
				if len(parts) == 2 {
					if decoded, err := url.PathUnescape(parts[1]); err == nil {
						t.Filename = SanitizeFilename(decoded)
					}
				}
			} else if fn, ok := params["filename"]; ok {
				t.Filename = SanitizeFilename(fn)
			}
		}
	}
	t.Category = DetectCategory(t.Filename)
	if t.SavePath != "" && t.Filename != "" {
		t.SavePath = filepath.Join(filepath.Dir(t.SavePath), t.Filename)
	}
}

// runWorker executes download for a single segment, with dynamic subdivision support.
func (t *Task) runWorker(seg *Segment, wg *sync.WaitGroup) {
	consecutiveErrors := 0
	for {
		if t.ctx.Err() != nil {
			return
		}

		if seg.CurrentOffset > seg.EndOffset && seg.EndOffset >= 0 {
			seg.Done = true
			seg.Active = false

			// Dynamic subdivision: see if another segment needs help
			newSeg, err := t.segmentMgr.SubdivideLargest()
			if err == nil && newSeg != nil {
				seg = newSeg
				continue // Take over the new subdivided segment
			}
			return
		}

		seg.Active = true
		req, err := http.NewRequestWithContext(t.ctx, "GET", t.URL, nil)
		if err != nil {
			seg.Error = err.Error()
			return
		}

		for k, v := range t.Headers {
			req.Header.Set(k, v)
		}
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 IFD/1.0")
		}

		if t.SupportsRange {
			if seg.EndOffset >= 0 {
				req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", seg.CurrentOffset, seg.EndOffset))
			} else {
				req.Header.Set("Range", fmt.Sprintf("bytes=%d-", seg.CurrentOffset))
			}
		}

		resp, err := t.client.Do(req)
		if err != nil {
			if t.ctx.Err() != nil {
				return
			}
			seg.Error = err.Error()
			consecutiveErrors++
			if consecutiveErrors > 5 {
				t.fail(fmt.Errorf("download error: %w", err))
				return
			}
			select {
			case <-t.ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
			continue
		}

		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			seg.Error = fmt.Sprintf("HTTP %d %s", resp.StatusCode, resp.Status)
			resp.Body.Close()
			t.fail(fmt.Errorf("server error: HTTP %d %s", resp.StatusCode, resp.Status))
			return
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			seg.Error = fmt.Sprintf("HTTP %d %s", resp.StatusCode, resp.Status)
			resp.Body.Close()
			consecutiveErrors++
			if consecutiveErrors > 5 {
				t.fail(fmt.Errorf("download failed: HTTP %d %s", resp.StatusCode, resp.Status))
				return
			}
			select {
			case <-t.ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
			continue
		}

		consecutiveErrors = 0

		if t.SupportsRange && resp.StatusCode == http.StatusOK {
			// Server does not support Range requests despite Range header
			t.mu.Lock()
			t.SupportsRange = false
			t.mu.Unlock()
			if seg.ID != 0 {
				resp.Body.Close()
				seg.Done = true
				seg.Active = false
				return
			}
			seg.EndOffset = -1
		}

		if t.TotalSize <= 0 && resp.ContentLength > 0 {
			t.TotalSize = resp.ContentLength
		}

		buf := make([]byte, 64*1024) // 64 KB chunk buffer
		for {
			if t.ctx.Err() != nil {
				resp.Body.Close()
				return
			}

			// Apply global & local bandwidth limits
			_ = t.engine.limiter.Wait(t.ctx, int64(len(buf)))
			if t.localLimiter != nil && t.SpeedLimit > 0 {
				_ = t.localLimiter.Wait(t.ctx, int64(len(buf)))
			}

			cur := atomic.LoadInt64(&seg.CurrentOffset)

			// Guard: If dynamic subdivision shortened our EndOffset, stop this connection cleanly
			if seg.EndOffset >= 0 && cur > seg.EndOffset {
				resp.Body.Close()
				seg.Done = true
				seg.Active = false
				break
			}

			readBuf := buf
			if seg.EndOffset >= 0 {
				rem := seg.EndOffset - cur + 1
				if rem > 0 && rem < int64(len(buf)) {
					readBuf = buf[:rem]
				}
			}

			n, readErr := resp.Body.Read(readBuf)
			if n > 0 {
				if _, writeErr := t.storage.WriteAt(readBuf[:n], cur); writeErr != nil {
					seg.Error = writeErr.Error()
					resp.Body.Close()
					t.fail(fmt.Errorf("storage write error: %w", writeErr))
					return
				}
				newCur := atomic.AddInt64(&seg.CurrentOffset, int64(n))
				if seg.EndOffset >= 0 && newCur > seg.EndOffset {
					resp.Body.Close()
					seg.Done = true
					seg.Active = false
					break
				}
			}

			if readErr != nil {
				resp.Body.Close()
				if readErr == io.EOF {
					seg.Done = true
					seg.Active = false

					// Check dynamic subdivision
					newSeg, err := t.segmentMgr.SubdivideLargest()
					if err == nil && newSeg != nil {
						seg = newSeg
						break // Download the newly split chunk
					}
					return
				}
				select {
				case <-t.ctx.Done():
					return
				case <-time.After(500 * time.Millisecond):
				}
				break
			}
		}
	}
}

// updateProgress computes transfer speed, ETA, and triggers telemetry update.
func (t *Task) updateProgress() {
	if t.segmentMgr == nil {
		return
	}

	now := time.Now()
	currentDownloaded := t.segmentMgr.TotalDownloaded()
	elapsed := now.Sub(t.prevTime).Seconds()

	if elapsed >= 0.5 {
		bytesDiff := currentDownloaded - t.prevBytes
		if bytesDiff < 0 {
			bytesDiff = 0
		}
		speed := int64(float64(bytesDiff) / elapsed)

		t.mu.Lock()
		t.Speed = speed
		if speed > 0 && t.TotalSize > 0 {
			remaining := t.TotalSize - currentDownloaded
			if remaining > 0 {
				t.ETA = remaining / speed
			} else {
				t.ETA = 0
			}
		} else {
			t.ETA = 0
		}
		t.prevBytes = currentDownloaded
		t.prevTime = now
		t.mu.Unlock()

		t.engine.updateTask(t)
	}
}

// Pause pauses active download.
func (t *Task) Pause() {
	t.mu.Lock()
	if t.Status != "Downloading" && t.Status != "Connecting" && t.Status != "Queued" {
		t.mu.Unlock()
		return
	}
	t.Status = "Paused"
	t.Speed = 0
	t.ETA = 0
	if t.cancel != nil {
		t.cancel()
	}
	t.mu.Unlock()

	t.engine.updateTask(t)
	t.engine.checkQueue()
}

// Resume resumes a paused download.
func (t *Task) Resume() {
	t.mu.Lock()
	if t.Status != "Paused" && t.Status != "Error" && t.Status != "Queued" {
		t.mu.Unlock()
		return
	}
	t.Status = "Queued"
	t.Error = ""
	t.mu.Unlock()

	t.engine.updateTask(t)
	go t.Start()
}

// Cancel cancels the download and optionally removes files.
func (t *Task) Cancel(deleteFiles bool) {
	t.Pause()

	t.mu.Lock()
	if t.storage != nil {
		_ = t.storage.Close()
	}
	if deleteFiles {
		// Retry removing files to handle Windows file handles gracefully
		go func(storage *Storage, finalPath, savePath string) {
			for i := 0; i < 5; i++ {
				time.Sleep(100 * time.Millisecond)
				if storage != nil {
					_ = storage.RemovePartFile()
				}
				if finalPath != "" {
					_ = os.Remove(finalPath)
				}
				if savePath != "" {
					_ = os.Remove(savePath)
				}
			}
		}(t.storage, t.FinalPath, t.SavePath)
	}
	t.mu.Unlock()

	t.engine.mu.Lock()
	delete(t.engine.tasks, t.ID)
	t.engine.mu.Unlock()

	if t.engine.QueueMgr != nil {
		t.engine.QueueMgr.RemoveTask(t.ID)
	}

	t.engine.store.DeleteRecord(t.ID)
	t.engine.checkQueue()
}

// Restart resets the download to 0 and re-downloads fresh from beginning.
func (t *Task) Restart() {
	t.Pause()

	t.mu.Lock()
	if t.storage != nil {
		_ = t.storage.RemovePartFile()
		t.storage = nil
	}
	if t.FinalPath != "" {
		_ = os.Remove(t.FinalPath)
		t.FinalPath = ""
	}
	t.segmentMgr = nil
	t.TotalSize = 0
	t.Speed = 0
	t.ETA = 0
	t.Status = "Queued"
	t.Error = ""
	t.CompletedAt = nil
	t.prevBytes = 0
	t.prevTime = time.Now()
	t.mu.Unlock()

	t.engine.updateTask(t)
	go t.Start()
}

// SetSpeedLimit dynamically updates task speed limit in bytes/sec (0 = unlimited).
func (t *Task) SetSpeedLimit(bytesPerSec int64) {
	t.mu.Lock()
	t.SpeedLimit = bytesPerSec
	if t.localLimiter == nil {
		t.localLimiter = NewRateLimiter(bytesPerSec)
	} else {
		t.localLimiter.SetLimit(bytesPerSec)
	}
	t.mu.Unlock()
	t.engine.updateTask(t)
}

// SetMaxConns updates the maximum connections for this task (1 to 32).
func (t *Task) SetMaxConns(conns int) {
	if conns < 1 {
		conns = 1
	}
	if conns > 32 {
		conns = 32
	}
	t.mu.Lock()
	t.MaxConns = conns
	t.mu.Unlock()
	t.engine.updateTask(t)
}

// Edit updates URL, filename, or save directory for an inactive download.
func (t *Task) Edit(newURL, newFilename, newSaveDir string) error {
	t.Pause()

	t.mu.Lock()
	if newURL != "" {
		parsed, err := url.Parse(newURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			t.mu.Unlock()
			return fmt.Errorf("invalid URL: %w", err)
		}
		t.URL = newURL
	}

	if newFilename != "" {
		t.Filename = SanitizeFilename(newFilename)
		t.Category = DetectCategory(t.Filename)
	}

	if newSaveDir != "" {
		t.SavePath = filepath.Join(newSaveDir, t.Filename)
	} else if newFilename != "" {
		t.SavePath = filepath.Join(filepath.Dir(t.SavePath), t.Filename)
	}
	t.mu.Unlock()

	t.engine.updateTask(t)
	return nil
}

func (t *Task) fail(err error) {
	t.mu.Lock()
	t.Status = "Error"
	t.Error = err.Error()
	t.Speed = 0
	t.ETA = 0
	t.mu.Unlock()

	t.engine.updateTask(t)
	t.engine.checkQueue()
}

// SetQueue moves the task to a different queue.
func (t *Task) SetQueue(queueID string) {
	t.mu.Lock()
	t.QueueID = queueID
	t.mu.Unlock()
	if t.engine.QueueMgr != nil {
		_ = t.engine.QueueMgr.MoveTask(t.ID, queueID)
	}
	t.engine.updateTask(t)
}

// ToRecord exports current task state as DownloadRecord.
func (t *Task) ToRecord() *DownloadRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var segments []*Segment
	var downloaded int64 = 0
	if t.segmentMgr != nil {
		segments = t.segmentMgr.GetAll()
		downloaded = t.segmentMgr.TotalDownloaded()
	}

	return &DownloadRecord{
		ID:              t.ID,
		URL:             t.URL,
		Filename:        t.Filename,
		SavePath:        t.SavePath,
		FinalPath:       t.FinalPath,
		TotalSize:       t.TotalSize,
		DownloadedBytes: downloaded,
		Status:          t.Status,
		Category:        t.Category,
		QueueID:         t.QueueID,
		QueueIndex:      t.QueueIndex,
		Speed:           t.Speed,
		SpeedLimit:      t.SpeedLimit,
		MaxConns:        t.MaxConns,
		ETA:             t.ETA,
		Segments:        segments,
		SupportsRange:   t.SupportsRange,
		Headers:         t.Headers,
		CreatedAt:       t.CreatedAt,
		CompletedAt:     t.CompletedAt,
		Error:           t.Error,
	}
}

func (e *Engine) updateTask(t *Task) {
	rec := t.ToRecord()
	e.store.UpdateRecord(rec)
	e.mu.RLock()
	cbs := make([]func(t *Task), len(e.updateCallbacks))
	copy(cbs, e.updateCallbacks)
	e.mu.RUnlock()
	for _, cb := range cbs {
		cb(t)
	}
}

func (e *Engine) saveTask(t *Task) {
	rec := t.ToRecord()
	e.store.UpdateRecord(rec)
}

// GetAllTasks returns a snapshot of all downloads.
func (e *Engine) GetAllTasks() []*DownloadRecord {
	e.mu.RLock()
	defer e.mu.RUnlock()

	res := make([]*DownloadRecord, 0, len(e.tasks))
	for _, t := range e.tasks {
		res = append(res, t.ToRecord())
	}
	return res
}

// GetTask returns a task by ID.
func (e *Engine) GetTask(id string) *Task {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.tasks[id]
}
