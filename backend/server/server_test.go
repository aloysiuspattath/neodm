package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ifd/backend/engine"
	"ifd/backend/grabber"
)

func setupTestServer(t *testing.T) (*Server, *engine.Engine, string) {
	tmpDir, err := os.MkdirTemp("", "ifd_server_test_*")
	if err != nil {
		t.Fatal(err)
	}

	store, err := engine.NewStoreWithPath(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	store.Settings.DefaultDownloadDir = tmpDir
	store.Settings.MaxConcurrentDownloads = 1

	eng := engine.NewEngine(store)
	hub := NewHub(eng)
	go hub.Run()

	srv := &Server{
		engine:        eng,
		store:         store,
		grabber:       grabber.NewSiteGrabber(),
		videoAnalyzer: grabber.NewVideoAnalyzer(),
		hub:           hub,
	}

	return srv, eng, tmpDir
}

// TestCORSSecurity validates allowed origins and blocks malicious web origins
func TestCORSSecurity(t *testing.T) {
	srv, _, tmpDir := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	handler := srv.cors(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	allowedOrigins := []string{
		"http://localhost:19765",
		"http://127.0.0.1:19765",
		"chrome-extension://abcdefghijklmnop",
		"moz-extension://12345678-abcd",
		"null",
		"",
	}

	for _, origin := range allowedOrigins {
		req := httptest.NewRequest("GET", "/api/downloads", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		handler(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Origin %q expected 200 OK, got %d", origin, rec.Code)
		}
	}

	blockedOrigins := []string{
		"https://evil.com",
		"http://attacker.org",
		"https://malicious-website.io",
	}

	for _, origin := range blockedOrigins {
		req := httptest.NewRequest("GET", "/api/downloads", nil)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		handler(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("Origin %q expected 403 Forbidden, got %d", origin, rec.Code)
		}
	}
}

// TestDownloadActionsAPI tests the full suite of action verbs
func TestDownloadActionsAPI(t *testing.T) {
	srv, eng, tmpDir := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	// Add a dummy blocker so tasks don't auto-download
	eng.AddTask("http://example.com/blocked", "blocker.zip", tmpDir, nil)

	task, err := eng.AddTask("http://example.com/file.bin", "file.bin", tmpDir, nil)
	if err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	callAction := func(actionReq ActionReq) int {
		body, _ := json.Marshal(actionReq)
		req := httptest.NewRequest("POST", "/api/download/action", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		srv.handleDownloadAction(rec, req)
		return rec.Code
	}

	// 1. Pause
	code := callAction(ActionReq{ID: task.ID, Action: "pause"})
	if code != http.StatusOK {
		t.Errorf("Pause returned %d, expected 200", code)
	}
	if task.Status != "Paused" {
		t.Errorf("Expected status 'Paused', got %s", task.Status)
	}

	// 2. Speed Limit
	code = callAction(ActionReq{ID: task.ID, Action: "speed_limit", SpeedLimit: 5 * 1024 * 1024})
	if code != http.StatusOK {
		t.Errorf("Speed limit returned %d, expected 200", code)
	}
	if task.SpeedLimit != 5*1024*1024 {
		t.Errorf("Expected speed limit 5MB/s, got %d", task.SpeedLimit)
	}

	// 3. Set Conns
	code = callAction(ActionReq{ID: task.ID, Action: "set_conns", MaxConns: 16})
	if code != http.StatusOK {
		t.Errorf("Set conns returned %d, expected 200", code)
	}
	if task.MaxConns != 16 {
		t.Errorf("Expected max conns 16, got %d", task.MaxConns)
	}

	// 4. Edit
	code = callAction(ActionReq{ID: task.ID, Action: "edit", NewURL: "http://example.com/updated.bin", Filename: "updated.bin", SaveDir: tmpDir})
	if code != http.StatusOK {
		t.Errorf("Edit returned %d, expected 200", code)
	}
	if task.Filename != "updated.bin" {
		t.Errorf("Expected filename updated.bin, got %s", task.Filename)
	}

	// 5. Renew URL
	code = callAction(ActionReq{ID: task.ID, Action: "renew_url", NewURL: "http://example.com/fresh-token-123.bin"})
	if code != http.StatusOK {
		t.Errorf("Renew URL returned %d, expected 200", code)
	}
	if task.URL != "http://example.com/fresh-token-123.bin" {
		t.Errorf("Expected URL to be renewed, got %s", task.URL)
	}

	// 6. Delete
	code = callAction(ActionReq{ID: task.ID, Action: "delete"})
	if code != http.StatusOK {
		t.Errorf("Delete returned %d, expected 200", code)
	}
	if eng.GetTask(task.ID) != nil {
		t.Errorf("Expected task %s to be deleted from engine", task.ID)
	}
}

// TestAddDownloadSanitization tests CRLF injection protection and path sanitization
func TestAddDownloadSanitization(t *testing.T) {
	srv, _, tmpDir := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	payload := AddDownloadReq{
		URL:      "http://example.com/testfile.iso",
		Filename: "../../Windows/malicious.exe",
		SaveDir:  tmpDir,
		Headers: map[string]string{
			"X-Injected\r\nHeader": "attack\r\nvalue",
		},
		Cookies: "session=123\r\nSet-Cookie: evil=456",
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/download/add", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleAddDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleAddDownload returned %d: %s", rec.Code, rec.Body.String())
	}

	var res map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	taskMap := res["task"].(map[string]interface{})

	// Filename should be sanitized to malicious.exe (traversal stripped)
	filename := taskMap["filename"].(string)
	if filename != "malicious.exe" {
		t.Errorf("Expected sanitized filename 'malicious.exe', got %s", filename)
	}

	// Headers should not contain \r or \n
	headers := taskMap["headers"].(map[string]interface{})
	for k, v := range headers {
		if strings.Contains(k, "\r") || strings.Contains(k, "\n") {
			t.Errorf("CRLF detected in header key: %q", k)
		}
		if strV, ok := v.(string); ok {
			if strings.Contains(strV, "\r") || strings.Contains(strV, "\n") {
				t.Errorf("CRLF detected in header value: %q", strV)
			}
		}
	}
}

// TestPayloadSizeLimit tests HTTP DoS protection on large request bodies
func TestPayloadSizeLimit(t *testing.T) {
	srv, _, tmpDir := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	// Create 2MB payload (> 1MB limit)
	hugeString := strings.Repeat("A", 2*1024*1024)
	payload := map[string]string{"huge": hugeString}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/api/download/add", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleAddDownload(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request on oversized payload, got %d", rec.Code)
	}
}

// TestWindowActionAPI tests the window management endpoint
func TestWindowActionAPI(t *testing.T) {
	srv, _, tmpDir := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	actions := []string{"minimize", "maximize", "drag"}
	for _, act := range actions {
		body, _ := json.Marshal(map[string]string{"action": act})
		req := httptest.NewRequest("POST", "/api/window/action", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		srv.handleWindowAction(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Window action %q returned %d, expected 200", act, rec.Code)
		}
	}
}

// TestQueueAPI tests queue listing, creation, starting, stopping, reordering, and deleting
func TestQueueAPI(t *testing.T) {
	srv, eng, tmpDir := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	// 1. List Queues
	req := httptest.NewRequest("GET", "/api/queues", nil)
	rec := httptest.NewRecorder()
	srv.handleListQueues(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleListQueues returned %d", rec.Code)
	}

	// 2. Create Queue
	createBody, _ := json.Marshal(CreateQueueReq{Name: "Documents Queue", Simultaneous: 1})
	req = httptest.NewRequest("POST", "/api/queue/create", bytes.NewReader(createBody))
	rec = httptest.NewRecorder()
	srv.handleCreateQueue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleCreateQueue returned %d: %s", rec.Code, rec.Body.String())
	}
	var createRes map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &createRes)
	qMap := createRes["queue"].(map[string]interface{})
	qID := qMap["id"].(string)

	// 3. Start Queue
	startBody, _ := json.Marshal(QueueIDReq{ID: qID})
	req = httptest.NewRequest("POST", "/api/queue/start", bytes.NewReader(startBody))
	rec = httptest.NewRecorder()
	srv.handleStartQueue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleStartQueue returned %d", rec.Code)
	}

	// 4. Stop Queue
	req = httptest.NewRequest("POST", "/api/queue/stop", bytes.NewReader(startBody))
	rec = httptest.NewRecorder()
	srv.handleStopQueue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleStopQueue returned %d", rec.Code)
	}

	// 5. Add a task to this queue and move it
	task, err := eng.AddTaskWithQueue("http://example.com/test.pdf", "test.pdf", tmpDir, nil, qID)
	if err != nil {
		t.Fatalf("failed to add task: %v", err)
	}
	moveBody, _ := json.Marshal(MoveTaskReq{TaskID: task.ID, TargetQueueID: "main_queue"})
	req = httptest.NewRequest("POST", "/api/queue/move_task", bytes.NewReader(moveBody))
	rec = httptest.NewRecorder()
	srv.handleMoveQueueTask(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleMoveQueueTask returned %d", rec.Code)
	}

	// 6. Delete Queue
	req = httptest.NewRequest("POST", "/api/queue/delete", bytes.NewReader(startBody))
	rec = httptest.NewRecorder()
	srv.handleDeleteQueue(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handleDeleteQueue returned %d", rec.Code)
	}
}

// TestBatchActionsAPI tests batch pause_all, resume_all, and delete_completed
func TestBatchActionsAPI(t *testing.T) {
	srv, eng, tmpDir := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	_, _ = eng.AddTask("http://example.com/a.bin", "a.bin", tmpDir, nil)
	_, _ = eng.AddTask("http://example.com/b.bin", "b.bin", tmpDir, nil)

	// Pause all
	body, _ := json.Marshal(BatchActionReq{Action: "pause_all"})
	req := httptest.NewRequest("POST", "/api/download/batch_action", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleBatchAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Batch pause_all returned %d", rec.Code)
	}

	// Pause selected
	tA := eng.GetAllTasks()[0]
	body, _ = json.Marshal(BatchActionReq{Action: "pause_selected", IDs: []string{tA.ID}})
	req = httptest.NewRequest("POST", "/api/download/batch_action", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	srv.handleBatchAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Batch pause_selected returned %d", rec.Code)
	}

	// Start selected
	body, _ = json.Marshal(BatchActionReq{Action: "start_selected", IDs: []string{tA.ID}})
	req = httptest.NewRequest("POST", "/api/download/batch_action", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	srv.handleBatchAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Batch start_selected returned %d", rec.Code)
	}

	// Resume all
	body, _ = json.Marshal(BatchActionReq{Action: "start_all"})
	req = httptest.NewRequest("POST", "/api/download/batch_action", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	srv.handleBatchAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Batch start_all returned %d", rec.Code)
	}

	// Delete all files
	body, _ = json.Marshal(BatchActionReq{Action: "delete_all_files"})
	req = httptest.NewRequest("POST", "/api/download/batch_action", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	srv.handleBatchAction(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Batch delete_all_files returned %d", rec.Code)
	}
}

// TestClipboardAPI tests the /api/clipboard/read endpoint
func TestClipboardAPI(t *testing.T) {
	srv, _, tmpDir := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	req := httptest.NewRequest("GET", "/api/clipboard/read", nil)
	rec := httptest.NewRecorder()
	srv.handleReadClipboard(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleReadClipboard returned %d", rec.Code)
	}

	var res map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if success, ok := res["success"].(bool); !ok || !success {
		t.Fatalf("Expected success true, got %v", res["success"])
	}
}

// TestAddDownloadImmediateDataTransfer verifies that adding a download starts data transfer immediately.
func TestAddDownloadImmediateDataTransfer(t *testing.T) {
	testPayload := []byte("Accelerated download data packet stream verification for IFD/NeoDM")
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testPayload)))
		w.WriteHeader(http.StatusOK)
		w.Write(testPayload)
	}))
	defer httpServer.Close()

	tmpDir, err := os.MkdirTemp("", "ifd_add_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := engine.NewStoreWithPath(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	store.Settings.DefaultDownloadDir = tmpDir
	store.Settings.MaxConcurrentDownloads = 4

	eng := engine.NewEngine(store)
	srv := NewServer(eng, store, nil, "127.0.0.1:0")

	completed := make(chan struct{})
	eng.OnTaskComplete(func(t *engine.Task) {
		select {
		case <-completed:
		default:
			close(completed)
		}
	})

	payload := AddDownloadReq{
		URL:      httpServer.URL + "/testdata.bin",
		Filename: "testdata.bin",
		SaveDir:  tmpDir,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/download/add", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleAddDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleAddDownload failed with %d: %s", rec.Code, rec.Body.String())
	}

	select {
	case <-completed:
	case <-time.After(5 * time.Second):
		t.Fatalf("Download did not complete in time")
	}

	tasks := eng.GetAllTasks()
	if len(tasks) == 0 {
		t.Fatalf("Expected at least 1 task")
	}
	task := tasks[0]
	if task.Status != "Completed" {
		t.Errorf("Expected status Completed, got %s", task.Status)
	}

	savedFile := filepath.Join(tmpDir, "testdata.bin")
	content, err := os.ReadFile(savedFile)
	if err != nil {
		t.Fatalf("Failed to read file on disk: %v", err)
	}
	if !bytes.Equal(content, testPayload) {
		t.Errorf("File content mismatch: expected %q, got %q", string(testPayload), string(content))
	}
}

func TestVideoEndpoints(t *testing.T) {
	srv, eng, tmpDir := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	videoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "2048")
		w.WriteHeader(http.StatusOK)
		w.Write(make([]byte, 2048))
	}))
	defer videoServer.Close()

	// 1. Analyze endpoint
	analyzeBody, _ := json.Marshal(VideoAnalyzeReq{URL: videoServer.URL + "/clip.mp4"})
	req := httptest.NewRequest("POST", "/api/video/analyze", bytes.NewReader(analyzeBody))
	rec := httptest.NewRecorder()
	srv.handleVideoAnalyze(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleVideoAnalyze failed with %d: %s", rec.Code, rec.Body.String())
	}

	var info grabber.VideoInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("Failed to decode video info: %v", err)
	}
	if len(info.Formats) == 0 {
		t.Errorf("Expected at least 1 video format")
	}

	// 2. Download endpoint
	dlBody, _ := json.Marshal(VideoDownloadReq{
		URL:      videoServer.URL + "/clip.mp4",
		Filename: "clip.mp4",
		SaveDir:  tmpDir,
		Quality:  "1080p",
	})
	req = httptest.NewRequest("POST", "/api/video/download", bytes.NewReader(dlBody))
	rec = httptest.NewRecorder()
	srv.handleVideoDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleVideoDownload failed with %d: %s", rec.Code, rec.Body.String())
	}

	tasks := eng.GetAllTasks()
	found := false
	for _, tk := range tasks {
		if tk.Filename == "clip.mp4" && tk.Category == "Videos" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected video task with Category Videos in engine")
	}

	// Test Video vs Audio extension enforcement
	// Case 1: Video selected with .mp3 input -> auto-corrected to .mp4
	vidPayload, _ := json.Marshal(VideoDownloadReq{
		URL:      "https://example.com/movie.mp4",
		Filename: "movie.mp3",
		Quality:  "1080p (Full HD Video + Audio)",
	})
	reqVid := httptest.NewRequest("POST", "/api/video/download", bytes.NewReader(vidPayload))
	recVid := httptest.NewRecorder()
	srv.handleVideoDownload(recVid, reqVid)
	if recVid.Code != http.StatusOK {
		t.Fatalf("handleVideoDownload failed: %s", recVid.Body.String())
	}

	// Case 2: Audio selected with .mp4 input -> auto-corrected to .mp3
	audPayload, _ := json.Marshal(VideoDownloadReq{
		URL:      "https://example.com/audio.mp4",
		Filename: "song.mp4",
		Quality:  "Audio Only (High Quality MP3)",
	})
	reqAud := httptest.NewRequest("POST", "/api/video/download", bytes.NewReader(audPayload))
	recAud := httptest.NewRecorder()
	srv.handleVideoDownload(recAud, reqAud)
	if recAud.Code != http.StatusOK {
		t.Fatalf("handleVideoDownload failed: %s", recAud.Body.String())
	}

	allTasks := eng.GetAllTasks()
	for _, tk := range allTasks {
		if strings.Contains(tk.URL, "movie.mp4") {
			if !strings.HasSuffix(tk.Filename, ".mp4") {
				t.Errorf("Expected .mp4 filename for 1080p video, got %s", tk.Filename)
			}
		}
		if strings.Contains(tk.URL, "audio.mp4") {
			if !strings.HasSuffix(tk.Filename, ".mp3") {
				t.Errorf("Expected .mp3 filename for Audio Only, got %s", tk.Filename)
			}
		}
	}
}

func TestSystemDrivesAPI(t *testing.T) {
	srv, _, tmpDir := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	req := httptest.NewRequest("GET", "/api/system/drives", nil)
	rec := httptest.NewRecorder()
	srv.handleSystemDrives(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleSystemDrives failed with %d: %s", rec.Code, rec.Body.String())
	}

	var res struct {
		Success bool                 `json:"success"`
		Drives  []engine.DriveMetric `json:"drives"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("Failed to parse system drives response: %v", err)
	}
	if !res.Success {
		t.Errorf("Expected success = true")
	}
	if len(res.Drives) == 0 {
		t.Errorf("Expected at least 1 drive metric")
	}
}

func TestExtensionEndpoints(t *testing.T) {
	srv, _, tmpDir := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	// 1. Test /api/extension/info
	req := httptest.NewRequest("GET", "/api/extension/info", nil)
	rec := httptest.NewRecorder()
	srv.handleExtensionInfo(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleExtensionInfo failed with %d: %s", rec.Code, rec.Body.String())
	}

	var infoRes struct {
		Path   string `json:"path"`
		Exists bool   `json:"exists"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &infoRes); err != nil {
		t.Fatalf("Failed to parse extension info: %v", err)
	}

	// 2. Test /api/open with folder
	openReqBody, _ := json.Marshal(OpenReq{Path: tmpDir, Type: "folder"})
	reqOpen := httptest.NewRequest("POST", "/api/open", bytes.NewReader(openReqBody))
	recOpen := httptest.NewRecorder()
	srv.handleOpenFileOrFolder(recOpen, reqOpen)

	if recOpen.Code != http.StatusOK {
		t.Fatalf("handleOpenFileOrFolder failed with %d: %s", recOpen.Code, recOpen.Body.String())
	}
}

func TestCheckFileExistsAPI(t *testing.T) {
	srv, _, tmpDir := setupTestServer(t)
	defer os.RemoveAll(tmpDir)

	// Test 1: File does not exist
	body, _ := json.Marshal(CheckFileExistsReq{
		URL:      "http://example.com/testfile.iso",
		Filename: "testfile.iso",
		SaveDir:  tmpDir,
	})
	req := httptest.NewRequest("POST", "/api/file/check_exists", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleCheckFileExists(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleCheckFileExists failed: %s", rec.Body.String())
	}

	var resp CheckFileExistsResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if resp.Exists {
		t.Errorf("Expected exists = false for non-existent file")
	}

	// Test 2: File exists on disk
	filePath := filepath.Join(tmpDir, "testfile.iso")
	if err := os.WriteFile(filePath, []byte("existing data"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/file/check_exists", bytes.NewReader(body))
	srv.handleCheckFileExists(rec2, req2)

	var resp2 CheckFileExistsResp
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("Failed to parse response 2: %v", err)
	}
	if !resp2.Exists {
		t.Errorf("Expected exists = true for existing file")
	}
	if resp2.ExistingFileSize != int64(len("existing data")) {
		t.Errorf("Expected size %d, got %d", len("existing data"), resp2.ExistingFileSize)
	}
	if resp2.SuggestedRename != "testfile (1).iso" {
		t.Errorf("Expected suggestedRename = 'testfile (1).iso', got '%s'", resp2.SuggestedRename)
	}
}

func TestMediaPlatformDetection(t *testing.T) {
	mediaURLs := []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ",
		"https://vimeo.com/12345678",
		"https://twitch.tv/streamer",
		"https://twitter.com/user/status/123",
		"https://x.com/user/status/123",
		"https://tiktok.com/@user/video/123",
		"https://example.com/live/playlist.m3u8",
		"https://example.com/stream/manifest.mpd",
	}
	for _, u := range mediaURLs {
		if !isMediaPlatformURL(u) {
			t.Errorf("Expected isMediaPlatformURL(%q) to be true", u)
		}
	}

	nonMediaURLs := []string{
		"https://example.com/file.zip",
		"https://download.mozilla.org/?product=firefox",
		"https://github.com/archive.tar.gz",
	}
	for _, u := range nonMediaURLs {
		if isMediaPlatformURL(u) {
			t.Errorf("Expected isMediaPlatformURL(%q) to be false", u)
		}
	}
}

func TestGenericVideoTitle(t *testing.T) {
	generic := []string{"", "watch", "video", "video.mp4", "media_download", "media_stream", "- YouTube", "youtube", "- Some Title"}
	for _, g := range generic {
		if !isGenericVideoTitle(g) {
			t.Errorf("Expected isGenericVideoTitle(%q) to be true", g)
		}
	}

	specific := []string{"Great Video Presentation", "My Podcast Episode 1"}
	for _, s := range specific {
		if isGenericVideoTitle(s) {
			t.Errorf("Expected isGenericVideoTitle(%q) to be false", s)
		}
	}
}

func TestMediaStreamAddDownload(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "neodm-media-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := engine.NewStoreWithPath(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.NewEngine(store)
	srv := NewServer(eng, store, nil, "127.0.0.1:0")

	payload := AddDownloadReq{
		URL:      "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Filename: "watch",
		SaveDir:  tmpDir,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/download/add", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleAddDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handleAddDownload failed with %d: %s", rec.Code, rec.Body.String())
	}

	tasks := eng.GetAllTasks()
	if len(tasks) != 1 {
		t.Fatalf("Expected 1 task, got %d", len(tasks))
	}
	task := tasks[0]
	// Verify that the original URL is preserved so yt-dlp receives the real video page
	if task.URL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Errorf("Expected URL to be preserved as YouTube page URL, got %s", task.URL)
	}
	if task.Category != "Videos" {
		t.Errorf("Expected Category to be 'Videos', got %s", task.Category)
	}
}
