package engine

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ifd/backend/grabber"
)

// TestSegmentPartitioning validates initial segment creation and boundaries
func TestSegmentPartitioning(t *testing.T) {
	totalSize := int64(100 * 1024 * 1024) // 100 MB
	count := 8

	sm := NewSegmentManager(totalSize, count)
	segments := sm.GetAll()

	if len(segments) != count {
		t.Fatalf("Expected %d segments, got %d", count, len(segments))
	}

	// Verify contiguous byte coverage without overlaps or gaps
	for i := 0; i < len(segments); i++ {
		if i == 0 && segments[i].StartOffset != 0 {
			t.Errorf("Segment 0 must start at 0, got %d", segments[i].StartOffset)
		}
		if i > 0 && segments[i].StartOffset != segments[i-1].EndOffset+1 {
			t.Errorf("Gap detected between segment %d and %d: prev end=%d, curr start=%d",
				i-1, i, segments[i-1].EndOffset, segments[i].StartOffset)
		}
	}

	last := segments[len(segments)-1]
	if last.EndOffset != totalSize-1 {
		t.Errorf("Final segment must end at %d, got %d", totalSize-1, last.EndOffset)
	}
}

// TestDynamicSubdivision validates IDM-style runtime chunk re-splitting
func TestDynamicSubdivision(t *testing.T) {
	totalSize := int64(40 * 1024 * 1024) // 40 MB
	sm := NewSegmentManager(totalSize, 4)

	// Simulate segment 0, 1, 2 finished, segment 3 still running with 10MB left
	segments := sm.GetAll()
	for i := 0; i < 3; i++ {
		segments[i].Done = true
		segments[i].Active = false
		segments[i].CurrentOffset = segments[i].EndOffset + 1
	}

	seg3 := sm.segments[3]
	seg3.Active = true
	seg3.Done = false
	seg3.CurrentOffset = seg3.StartOffset // 0 bytes downloaded in chunk 3

	oldEnd := seg3.EndOffset
	initialRemaining := seg3.RemainingBytes()

	// Subdivide largest active segment
	newSeg, err := sm.SubdivideLargest()
	if err != nil {
		t.Fatalf("Expected successful subdivision, got error: %v", err)
	}

	if newSeg == nil {
		t.Fatal("Expected new segment to be returned")
	}

	// Verify updated boundaries
	if seg3.EndOffset >= oldEnd {
		t.Errorf("Original segment end offset was not reduced: got %d, original was %d", seg3.EndOffset, oldEnd)
	}

	if newSeg.StartOffset != seg3.EndOffset+1 {
		t.Errorf("New segment start offset %d does not follow original segment end %d", newSeg.StartOffset, seg3.EndOffset)
	}

	if newSeg.EndOffset != oldEnd {
		t.Errorf("New segment must cover through original end %d, got %d", oldEnd, newSeg.EndOffset)
	}

	combinedRemaining := seg3.RemainingBytes() + newSeg.RemainingBytes()
	if combinedRemaining != initialRemaining {
		t.Errorf("Total remaining bytes mismatch: original %d vs combined %d", initialRemaining, combinedRemaining)
	}
}

// TestRateLimiter validates token bucket bandwidth throttling
func TestRateLimiter(t *testing.T) {
	limitBytesPerSec := int64(1024 * 1024) // 1 MB/s
	limiter := NewRateLimiter(limitBytesPerSec)

	ctx := context.Background()

	// Consuming within capacity should not block significantly
	start := time.Now()
	err := limiter.Wait(ctx, 100*1024) // 100 KB
	if err != nil {
		t.Fatalf("Limiter wait failed: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Errorf("Wait for tokens within burst limit took too long: %v", elapsed)
	}
}

// TestStorageDirectOffsetWrite validates WriteAt and Finalize
func TestStorageDirectOffsetWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ifd_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	targetFile := filepath.Join(tmpDir, "testfile.bin")
	totalSize := int64(1024)

	storage, err := NewStorage(targetFile, totalSize, "test_1")
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	// Write at offset 0
	data1 := []byte("HELLO")
	n1, err := storage.WriteAt(data1, 0)
	if err != nil || n1 != len(data1) {
		t.Fatalf("WriteAt offset 0 failed: %v", err)
	}

	// Write at offset 500
	data2 := []byte("WORLD")
	n2, err := storage.WriteAt(data2, 500)
	if err != nil || n2 != len(data2) {
		t.Fatalf("WriteAt offset 500 failed: %v", err)
	}

	finalPath, err := storage.Finalize()
	if err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	// Verify contents on disk
	content, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("Failed to read finalized file: %v", err)
	}

	if string(content[:5]) != "HELLO" {
		t.Errorf("Expected 'HELLO' at offset 0, got %s", string(content[:5]))
	}
	if string(content[500:505]) != "WORLD" {
		t.Errorf("Expected 'WORLD' at offset 500, got %s", string(content[500:505]))
	}
}

// TestBatchUrlExpansion validates template expansion
func TestBatchUrlExpansion(t *testing.T) {
	template := "https://example.com/file_[01-05].zip"
	expanded := grabber.ExpandBatchPattern(template)

	if len(expanded) != 5 {
		t.Fatalf("Expected 5 expanded URLs, got %d", len(expanded))
	}

	expectedFirst := "https://example.com/file_01.zip"
	expectedLast := "https://example.com/file_05.zip"

	if expanded[0] != expectedFirst {
		t.Errorf("Expected first URL %s, got %s", expectedFirst, expanded[0])
	}
	if expanded[4] != expectedLast {
		t.Errorf("Expected last URL %s, got %s", expectedLast, expanded[4])
	}
}

// TestSanitizeFilenamePenTest validates defense against path traversal and malicious file names
func TestSanitizeFilenamePenTest(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"../../Windows/System32/cmd.exe", "cmd.exe"},
		{"..\\..\\Windows\\calc.exe", "calc.exe"},
		{"CON.txt", "_CON.txt"},
		{"prn.pdf", "_prn.pdf"},
		{"nul", "_nul"},
		{"foo:bar*baz?.zip", "foo_bar_baz_.zip"},
		{"  normal_file.iso  ", "normal_file.iso"},
		{"", "download"},
		{"...", "download"},
	}

	for _, c := range cases {
		out := SanitizeFilename(c.input)
		if out != c.expected {
			t.Errorf("SanitizeFilename(%q) = %q, expected %q", c.input, out, c.expected)
		}
	}
}

// TestTaskControlActions validates Pause, Resume, Restart, SpeedLimit, MaxConns, and Edit
func TestTaskControlActions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ifd_task_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStoreWithPath(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	store.Settings.DefaultDownloadDir = tmpDir
	// Set concurrency limit to 1 and add an active dummy task so checkQueue doesn't auto-start network calls
	store.Settings.MaxConcurrentDownloads = 1

	eng := NewEngine(store)
	eng.tasks["dummy_blocker"] = &Task{
		ID:     "dummy_blocker",
		Status: "Downloading",
	}

	task := &Task{
		ID:           "test_ctrl_task",
		URL:          "https://example.com/archive.zip",
		Filename:     "archive.zip",
		SavePath:     filepath.Join(tmpDir, "archive.zip"),
		Status:       "Downloading",
		Category:     "Compressed",
		MaxConns:     8,
		SpeedLimit:   0,
		CreatedAt:    time.Now(),
		engine:       eng,
		client:       eng.client,
		localLimiter: NewRateLimiter(0),
	}
	eng.mu.Lock()
	eng.tasks[task.ID] = task
	eng.mu.Unlock()

	// 1. Test Pause
	task.Pause()
	if task.Status != "Paused" {
		t.Errorf("Expected status 'Paused', got %s", task.Status)
	}

	// 2. Test Resume
	task.Resume()
	if task.Status != "Queued" && task.Status != "Connecting" {
		t.Errorf("Expected status 'Queued' or 'Connecting', got %s", task.Status)
	}
	task.Pause()

	// 3. Test Speed Limit
	task.SetSpeedLimit(2 * 1024 * 1024)
	if task.SpeedLimit != 2*1024*1024 {
		t.Errorf("Expected speed limit %d, got %d", 2*1024*1024, task.SpeedLimit)
	}

	// 4. Test Max Connections
	task.SetMaxConns(16)
	if task.MaxConns != 16 {
		t.Errorf("Expected max conns 16, got %d", task.MaxConns)
	}

	// 5. Test Edit
	err = task.Edit("https://example.com/new_archive.zip", "renamed.zip", tmpDir)
	if err != nil {
		t.Fatalf("Edit failed: %v", err)
	}
	if task.Filename != "renamed.zip" {
		t.Errorf("Expected filename 'renamed.zip', got %s", task.Filename)
	}
	if task.URL != "https://example.com/new_archive.zip" {
		t.Errorf("Expected new URL, got %s", task.URL)
	}

	// 6. Test Cancel
	task.Cancel(false)
	if eng.GetTask(task.ID) != nil {
		t.Errorf("Expected task %s to be removed from engine", task.ID)
	}
}

// TestRealHTTPDownloadTransfer verifies end-to-end data transfer with a real HTTP test server.
func TestRealHTTPDownloadTransfer(t *testing.T) {
	testData := []byte("Hello, NeoDM accelerated multi-connection turbo engine download verification test data!")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader != "" {
			var start, end int64
			n, _ := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end)
			if n == 2 {
				if end >= int64(len(testData)) {
					end = int64(len(testData)) - 1
				}
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(testData)))
				w.Header().Set("Accept-Ranges", "bytes")
				w.WriteHeader(http.StatusPartialContent)
				w.Write(testData[start : end+1])
				return
			} else if n == 1 {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(testData)-1, len(testData)))
				w.Header().Set("Accept-Ranges", "bytes")
				w.WriteHeader(http.StatusPartialContent)
				w.Write(testData[start:])
				return
			}
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testData)))
		w.WriteHeader(http.StatusOK)
		w.Write(testData)
	}))
	defer server.Close()

	tmpDir, err := os.MkdirTemp("", "ifd_download_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStoreWithPath(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	store.Settings.DefaultDownloadDir = tmpDir

	eng := NewEngine(store)

	var updateCount int
	completed := make(chan struct{})

	eng.OnTaskUpdate(func(task *Task) {
		updateCount++
	})
	eng.OnTaskComplete(func(task *Task) {
		select {
		case <-completed:
		default:
			close(completed)
		}
	})

	task, err := eng.AddTask(server.URL+"/testfile.bin", "downloaded_file.bin", tmpDir, nil)
	if err != nil {
		t.Fatalf("AddTask failed: %v", err)
	}

	select {
	case <-completed:
	case <-time.After(5 * time.Second):
		t.Fatalf("Download timed out. Task status: %s, Error: %s", task.Status, task.Error)
	}

	if task.Status != "Completed" {
		t.Errorf("Expected status Completed, got %s (err: %s)", task.Status, task.Error)
	}

	if updateCount == 0 {
		t.Errorf("Expected OnTaskUpdate callbacks to be fired, got 0")
	}

	// Verify file content on disk
	content, err := os.ReadFile(task.FinalPath)
	if err != nil {
		t.Fatalf("Failed to read completed file at %s: %v", task.FinalPath, err)
	}
	if !bytes.Equal(content, testData) {
		t.Errorf("Content mismatch: expected %q, got %q", string(testData), string(content))
	}
}

