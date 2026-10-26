package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"ifd/backend/engine"
	"ifd/backend/grabber"
	"ifd/backend/platform"
)

// Server coordinates the HTTP REST API, WebSocket telemetry, and static frontend.
type Server struct {
	engine        *engine.Engine
	store         *engine.Store
	grabber       *grabber.SiteGrabber
	videoAnalyzer *grabber.VideoAnalyzer
	hub           *Hub
	frontend      fs.FS
	addr          string
}

// NewServer creates a new API and UI server.
func NewServer(eng *engine.Engine, store *engine.Store, frontendFS fs.FS, addr string) *Server {
	hub := NewHub(eng)
	go hub.Run()

	s := &Server{
		engine:        eng,
		store:         store,
		grabber:       grabber.NewSiteGrabber(),
		videoAnalyzer: grabber.NewVideoAnalyzer(),
		hub:           hub,
		frontend:      frontendFS,
		addr:          addr,
	}

	eng.OnTaskUpdate(func(t *engine.Task) {
		hub.BroadcastEvent("task_updated", t.ToRecord())
	})
	eng.OnTaskComplete(func(t *engine.Task) {
		hub.BroadcastEvent("task_completed", t.ToRecord())
	})

	return s
}

// Hub returns the server's WebSocket hub.
func (s *Server) Hub() *Hub {
	return s.hub
}

// Start launches the HTTP server listening on the configured address.
func (s *Server) Start() error {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	return s.StartWithListener(l)
}

// StartWithListener serves HTTP using the provided listener.
func (s *Server) StartWithListener(l net.Listener) error {
	mux := http.NewServeMux()

	// WebSocket endpoint
	mux.HandleFunc("/ws", s.hub.HandleWS)

	// REST API routes
	mux.HandleFunc("/api/download/add", s.cors(s.handleAddDownload))
	mux.HandleFunc("/api/downloads", s.cors(s.handleListDownloads))
	mux.HandleFunc("/api/download/action", s.cors(s.handleDownloadAction))
	mux.HandleFunc("/api/download/batch_action", s.cors(s.handleBatchAction))
	mux.HandleFunc("/api/queues", s.cors(s.handleListQueues))
	mux.HandleFunc("/api/queue/create", s.cors(s.handleCreateQueue))
	mux.HandleFunc("/api/queue/delete", s.cors(s.handleDeleteQueue))
	mux.HandleFunc("/api/queue/start", s.cors(s.handleStartQueue))
	mux.HandleFunc("/api/queue/stop", s.cors(s.handleStopQueue))
	mux.HandleFunc("/api/queue/reorder", s.cors(s.handleReorderQueue))
	mux.HandleFunc("/api/queue/move_task", s.cors(s.handleMoveQueueTask))
	mux.HandleFunc("/api/grabber/crawl", s.cors(s.handleGrabberCrawl))
	mux.HandleFunc("/api/grabber/batch", s.cors(s.handleGrabberBatch))
	mux.HandleFunc("/api/video/analyze", s.cors(s.handleVideoAnalyze))
	mux.HandleFunc("/api/video/download", s.cors(s.handleVideoDownload))
	mux.HandleFunc("/api/settings", s.cors(s.handleSettings))
	mux.HandleFunc("/api/open", s.cors(s.handleOpenFileOrFolder))
	mux.HandleFunc("/api/extension/info", s.cors(s.handleExtensionInfo))
	mux.HandleFunc("/api/extension/open", s.cors(s.handleExtensionOpen))
	mux.HandleFunc("/api/extension/register", s.cors(s.handleExtensionRegister))
	mux.HandleFunc("/api/select_folder", s.cors(s.handleSelectFolder))
	mux.HandleFunc("/api/folder/select", s.cors(s.handleSelectFolder))
	mux.HandleFunc("/api/system/drives", s.cors(s.handleSystemDrives))
	mux.HandleFunc("/api/window/action", s.cors(s.handleWindowAction))
	mux.HandleFunc("/api/clipboard/read", s.cors(s.handleReadClipboard))

	// Static frontend
	if s.frontend != nil {
		fileServer := http.FileServer(http.FS(s.frontend))
		mux.Handle("/", fileServer)
	}

	return http.Serve(l, mux)
}

func isAllowedOrigin(origin string) bool {
	if origin == "" || origin == "null" {
		return true
	}
	if strings.HasPrefix(origin, "chrome-extension://") ||
		strings.HasPrefix(origin, "moz-extension://") ||
		strings.HasPrefix(origin, "http://127.0.0.1") ||
		strings.HasPrefix(origin, "http://localhost") ||
		strings.HasPrefix(origin, "https://127.0.0.1") ||
		strings.HasPrefix(origin, "https://localhost") {
		return true
	}
	return false
}

func (s *Server) cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && !isAllowedOrigin(origin) {
			http.Error(w, "Cross-origin request forbidden", http.StatusForbidden)
			return
		}

		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		h(w, r)
	}
}

type AddDownloadReq struct {
	URL       string            `json:"url"`
	Filename  string            `json:"filename"`
	SaveDir   string            `json:"saveDir"`
	QueueID   string            `json:"queueId,omitempty"`
	QueueName string            `json:"queueName,omitempty"`
	Headers   map[string]string `json:"headers"`
	Cookies   string            `json:"cookies"`
	AutoStart *bool             `json:"autoStart,omitempty"`
}

func (s *Server) handleAddDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	var req AddDownloadReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cleanHeaders := make(map[string]string)
	for k, v := range req.Headers {
		cleanK := strings.ReplaceAll(strings.ReplaceAll(k, "\r", ""), "\n", "")
		cleanV := strings.ReplaceAll(strings.ReplaceAll(v, "\r", ""), "\n", "")
		cleanHeaders[cleanK] = cleanV
	}
	if req.Cookies != "" {
		cleanCookies := strings.ReplaceAll(strings.ReplaceAll(req.Cookies, "\r", ""), "\n", "")
		cleanHeaders["Cookie"] = cleanCookies
	}

	targetQueue := req.QueueID
	if targetQueue == "" && req.QueueName != "" {
		targetQueue = req.QueueName
	}
	if targetQueue == "" {
		targetQueue = "main_queue"
	}

	downloadURL := req.URL
	downloadFilename := req.Filename
	if strings.Contains(req.URL, "youtube.com") || strings.Contains(req.URL, "youtu.be") {
		if s.videoAnalyzer != nil {
			if vInfo, vErr := s.videoAnalyzer.Analyze(r.Context(), req.URL); vErr == nil && vInfo.BestURL != "" {
				downloadURL = vInfo.BestURL
				if downloadFilename == "" || strings.HasPrefix(downloadFilename, "-") {
					downloadFilename = vInfo.Filename
				}
			}
		}
	}

	task, err := s.engine.AddTaskWithQueue(downloadURL, downloadFilename, req.SaveDir, cleanHeaders, targetQueue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Ensure immediate data transfer when autoStart is requested (default true)
	if req.AutoStart == nil || *req.AutoStart {
		if s.engine.QueueMgr != nil {
			if q := s.engine.QueueMgr.GetQueue(targetQueue); q != nil && !q.ScheduleEnabled {
				_ = s.engine.QueueMgr.StartQueue(targetQueue)
			}
		}
		if task.Status == "Queued" {
			go task.Start()
		}
	}

	s.hub.BroadcastEvent("task_added", task.ToRecord())

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"task":    task.ToRecord(),
	})
}

func (s *Server) handleListDownloads(w http.ResponseWriter, r *http.Request) {
	tasks := s.engine.GetAllTasks()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"downloads": tasks,
	})
}

type ActionReq struct {
	ID         string `json:"id"`
	Action     string `json:"action"` // "pause", "resume", "restart", "delete", "delete_file", "speed_limit", "set_conns", "edit", "move_queue"
	QueueID    string `json:"queueId,omitempty"`
	SpeedLimit int64  `json:"speedLimit,omitempty"`
	MaxConns   int    `json:"maxConns,omitempty"`
	NewURL     string `json:"newUrl,omitempty"`
	Filename   string `json:"filename,omitempty"`
	SaveDir    string `json:"saveDir,omitempty"`
}

func (s *Server) handleDownloadAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	var req ActionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	task := s.engine.GetTask(req.ID)
	if task == nil {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	switch req.Action {
	case "pause":
		task.Pause()
	case "resume":
		task.Resume()
	case "restart":
		task.Restart()
	case "delete", "cancel":
		task.Cancel(false)
		s.hub.BroadcastEvent("task_deleted", map[string]string{"id": req.ID})
	case "delete_file":
		task.Cancel(true)
		s.hub.BroadcastEvent("task_deleted", map[string]string{"id": req.ID})
	case "speed_limit":
		task.SetSpeedLimit(req.SpeedLimit)
	case "set_conns":
		task.SetMaxConns(req.MaxConns)
	case "move_queue":
		task.SetQueue(req.QueueID)
	case "edit", "renew_url":
		if err := task.Edit(req.NewURL, req.Filename, req.SaveDir); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Action == "renew_url" {
			task.Resume()
		}
	default:
		http.Error(w, "Unknown action", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

type BatchActionReq struct {
	Action     string   `json:"action"` // "start_all", "pause_all", "delete_completed", "delete_selected", "start_selected", "pause_selected", "move_queue_selected", "speed_limit_selected"
	IDs        []string `json:"ids,omitempty"`
	QueueID    string   `json:"queueId,omitempty"`
	SpeedLimit int64    `json:"speedLimit,omitempty"`
}

func (s *Server) handleBatchAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BatchActionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	allTasks := s.engine.GetAllTasks()
	switch req.Action {
	case "start_all", "resume_all":
		for _, rec := range allTasks {
			if rec.Status == "Paused" || rec.Status == "Error" || rec.Status == "Queued" {
				t := s.engine.GetTask(rec.ID)
				if t != nil {
					t.Resume()
				}
			}
		}
	case "pause_all":
		for _, rec := range allTasks {
			if rec.Status == "Downloading" || rec.Status == "Connecting" || rec.Status == "Queued" {
				t := s.engine.GetTask(rec.ID)
				if t != nil {
					t.Pause()
				}
			}
		}
	case "start_selected", "resume_selected":
		for _, id := range req.IDs {
			t := s.engine.GetTask(id)
			if t != nil {
				t.Resume()
			}
		}
	case "pause_selected":
		for _, id := range req.IDs {
			t := s.engine.GetTask(id)
			if t != nil {
				t.Pause()
			}
		}
	case "move_queue_selected":
		if req.QueueID != "" {
			for _, id := range req.IDs {
				t := s.engine.GetTask(id)
				if t != nil {
					t.SetQueue(req.QueueID)
				}
			}
		}
	case "speed_limit_selected":
		for _, id := range req.IDs {
			t := s.engine.GetTask(id)
			if t != nil {
				t.SetSpeedLimit(req.SpeedLimit)
			}
		}
	case "delete_completed":
		for _, rec := range allTasks {
			if rec.Status == "Completed" {
				t := s.engine.GetTask(rec.ID)
				if t != nil {
					t.Cancel(false)
					s.hub.BroadcastEvent("task_deleted", map[string]string{"id": rec.ID})
				}
			}
		}
	case "delete_selected":
		for _, id := range req.IDs {
			t := s.engine.GetTask(id)
			if t != nil {
				t.Cancel(false)
				s.hub.BroadcastEvent("task_deleted", map[string]string{"id": id})
			}
		}
	case "delete_all_files":
		for _, rec := range allTasks {
			t := s.engine.GetTask(rec.ID)
			if t != nil {
				t.Cancel(true)
				s.hub.BroadcastEvent("task_deleted", map[string]string{"id": rec.ID})
			}
		}
	case "delete_selected_files":
		for _, id := range req.IDs {
			t := s.engine.GetTask(id)
			if t != nil {
				t.Cancel(true)
				s.hub.BroadcastEvent("task_deleted", map[string]string{"id": id})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (s *Server) handleListQueues(w http.ResponseWriter, r *http.Request) {
	if s.engine.QueueMgr == nil {
		http.Error(w, "Queue manager not initialized", http.StatusInternalServerError)
		return
	}
	queues := s.engine.QueueMgr.GetAllQueues()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"queues":  queues,
	})
}

type CreateQueueReq struct {
	Name         string `json:"name"`
	Simultaneous int    `json:"simultaneous"`
}

func (s *Server) handleCreateQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req CreateQueueReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "Queue name cannot be empty", http.StatusBadRequest)
		return
	}
	q, err := s.engine.QueueMgr.CreateQueue(req.Name, req.Simultaneous)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "queue": q})
}

type QueueIDReq struct {
	ID string `json:"id"`
}

func (s *Server) handleDeleteQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req QueueIDReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.engine.QueueMgr.DeleteQueue(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (s *Server) handleStartQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req QueueIDReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.engine.QueueMgr.StartQueue(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (s *Server) handleStopQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req QueueIDReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.engine.QueueMgr.StopQueue(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

type ReorderQueueReq struct {
	ID    string   `json:"id"`
	Order []string `json:"order"`
}

func (s *Server) handleReorderQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req ReorderQueueReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.engine.QueueMgr.ReorderQueue(req.ID, req.Order); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

type MoveTaskReq struct {
	TaskID        string `json:"taskId"`
	TargetQueueID string `json:"targetQueueId"`
}

func (s *Server) handleMoveQueueTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req MoveTaskReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.engine.QueueMgr.MoveTask(req.TaskID, req.TargetQueueID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (s *Server) handleGrabberCrawl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var opts grabber.GrabberOptions
	if err := json.NewDecoder(r.Body).Decode(&opts); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if opts.Depth <= 0 {
		opts.Depth = 1
	}

	links, err := s.grabber.Crawl(opts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"links":   links,
		"count":   len(links),
	})
}

type BatchReq struct {
	Template string `json:"template"`
	SaveDir  string `json:"saveDir"`
}

func (s *Server) handleGrabberBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BatchReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	urls := grabber.ExpandBatchPattern(req.Template)
	for _, u := range urls {
		_, _ = s.engine.AddTask(u, "", req.SaveDir, nil)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"count":   len(urls),
	})
}

type VideoAnalyzeReq struct {
	URL string `json:"url"`
}

type VideoDownloadReq struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	SaveDir  string `json:"saveDir"`
	Quality  string `json:"quality"`
}

func (s *Server) handleVideoAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req VideoAnalyzeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	analyzer := s.videoAnalyzer
	if analyzer == nil {
		analyzer = grabber.NewVideoAnalyzer()
	}
	info, err := analyzer.Analyze(r.Context(), req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

func (s *Server) handleVideoDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req VideoDownloadReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	downloadURL := req.URL
	downloadFilename := req.Filename

	// If req.URL is still a page URL (e.g. youtube.com), resolve to real direct stream URL
	if strings.Contains(req.URL, "youtube.com") || strings.Contains(req.URL, "youtu.be") {
		if s.videoAnalyzer != nil {
			if vInfo, vErr := s.videoAnalyzer.Analyze(r.Context(), req.URL); vErr == nil && vInfo.BestURL != "" {
				downloadURL = vInfo.BestURL
				if downloadFilename == "" || strings.HasPrefix(downloadFilename, "-") {
					downloadFilename = vInfo.Filename
				}
			}
		}
	}

	saveDir := req.SaveDir
	if saveDir == "" {
		if s.store.Settings != nil && s.store.Settings.CategoryFolders != nil {
			saveDir = s.store.Settings.CategoryFolders["Videos"]
		}
		if saveDir == "" && s.store.Settings != nil {
			saveDir = s.store.Settings.DefaultDownloadDir
		}
	}

	task, err := s.engine.AddTask(downloadURL, downloadFilename, saveDir, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	task.Category = "Videos"
	task.StreamQuality = req.Quality

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"taskId":  task.ID,
		"message": "Video download started",
	})
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.store.Settings)
		return
	}

	if r.Method == http.MethodPost {
		var updated engine.Settings
		if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.store.Settings = &updated
		_ = s.store.SaveSettings()
		s.engine.SetGlobalSpeedLimit(updated.GlobalSpeedLimit)

		// Sync Windows auto-start setting
		_ = platform.SetAutoStart(updated.AutoStart)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"settings": s.store.Settings,
		})
	}
}

type OpenReq struct {
	Path string `json:"path"`
	Type string `json:"type"` // "file" or "folder"
}

func (s *Server) handleOpenFileOrFolder(w http.ResponseWriter, r *http.Request) {
	var req OpenReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	go func() {
		cleanPath := filepath.Clean(req.Path)
		if req.Type == "folder" {
			// If file exists on disk, reveal it and highlight in Windows Explorer
			if fi, err := os.Stat(cleanPath); err == nil && !fi.IsDir() {
				cmd := exec.Command("explorer.exe")
				cmd.SysProcAttr = &syscall.SysProcAttr{
					CmdLine: fmt.Sprintf(`explorer.exe /select,"%s"`, cleanPath),
				}
				_ = cmd.Start()
			} else {
				dir := cleanPath
				if fi, err := os.Stat(cleanPath); err != nil || !fi.IsDir() {
					dir = filepath.Dir(cleanPath)
				}
				_ = os.MkdirAll(dir, 0755)
				cmd := exec.Command("explorer.exe")
				cmd.SysProcAttr = &syscall.SysProcAttr{
					CmdLine: fmt.Sprintf(`explorer.exe "%s"`, dir),
				}
				_ = cmd.Start()
			}
		} else {
			// Launch file with Windows default associated application silently
			cmd := exec.Command("cmd.exe")
			cmd.SysProcAttr = &syscall.SysProcAttr{
				CmdLine:    fmt.Sprintf(`cmd.exe /c start "" "%s"`, cleanPath),
				HideWindow: true,
			}
			_ = cmd.Start()
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (s *Server) handleExtensionInfo(w http.ResponseWriter, r *http.Request) {
	extDir := platform.GetExtensionDir()
	exists := false
	if extDir != "" {
		if _, err := os.Stat(filepath.Join(extDir, "manifest.json")); err == nil {
			exists = true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"path":   extDir,
		"exists": exists,
	})
}

func (s *Server) handleExtensionOpen(w http.ResponseWriter, r *http.Request) {
	extDir := platform.GetExtensionDir()
	if extDir != "" {
		_ = os.MkdirAll(extDir, 0755)
		manifest := filepath.Join(extDir, "manifest.json")
		cmd := exec.Command("explorer.exe")
		if _, err := os.Stat(manifest); err == nil {
			cmd.SysProcAttr = &syscall.SysProcAttr{
				CmdLine: fmt.Sprintf(`explorer.exe /select,"%s"`, manifest),
			}
		} else {
			cmd.SysProcAttr = &syscall.SysProcAttr{
				CmdLine: fmt.Sprintf(`explorer.exe "%s"`, extDir),
			}
		}
		_ = cmd.Start()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (s *Server) handleExtensionRegister(w http.ResponseWriter, r *http.Request) {
	extDir := platform.GetExtensionDir()
	err := platform.RegisterBrowserExtensions(extDir)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Browser extension registered for Chrome, Edge, Brave, and Firefox.",
	})
}

type WindowActionReq struct {
	Action string `json:"action"` // "minimize", "maximize", "close", "drag"
}

func (s *Server) handleWindowAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req WindowActionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	platform.PerformWindowAction(0, req.Action)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (s *Server) handleSelectFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	psScript := `Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.FolderBrowserDialog; $f.Description = 'Select Download Directory'; if ($f.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $f.SelectedPath }`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	selectedPath := strings.TrimSpace(string(out))
	if err != nil || selectedPath == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "cancelled": true})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "path": selectedPath})
}

func (s *Server) handleReadClipboard(w http.ResponseWriter, r *http.Request) {
	text := platform.GetClipboardText()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"text":    text,
	})
}

func (s *Server) handleSystemDrives(w http.ResponseWriter, r *http.Request) {
	drives, err := engine.GetDriveMetrics()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"drives":  drives,
	})
}
