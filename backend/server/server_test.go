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
		engine: eng,
		store:  store,
		hub:    hub,
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
