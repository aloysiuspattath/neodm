package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Settings contains global IFD configurations.
type Settings struct {
	MaxConcurrentDownloads    int                     `json:"maxConcurrentDownloads"`
	MaxConnectionsPerDownload int                     `json:"maxConnectionsPerDownload"`
	GlobalSpeedLimit          int64                   `json:"globalSpeedLimit"` // bytes/sec, 0 = unlimited
	DefaultDownloadDir        string                  `json:"defaultDownloadDir"`
	TemporaryDirectory        string                  `json:"temporaryDirectory,omitempty"`
	UseTargetForTemp          bool                    `json:"useTargetForTemp"`
	CategoryFolders           map[string]string       `json:"categoryFolders"`
	AutoStart                 bool                    `json:"autoStart"`
	AutoFetchEnabled          bool                    `json:"autoFetchEnabled"`
	InterceptExtensions       []string                `json:"interceptExtensions"`
	MinInterceptSizeBytes     int64                   `json:"minInterceptSizeBytes"`
	SoundAlertOnComplete      bool                    `json:"soundAlertOnComplete"`
	DarkTheme                 bool                    `json:"darkTheme"`
	SelectedTheme             string                  `json:"selectedTheme"` // "dark" (Bento Dark), "light" (Bento Light)
	StartMinimized            bool                    `json:"startMinimized"`
	AutoOpenCompleted         bool                    `json:"autoOpenCompleted"`
	NetworkBufferSize         int                     `json:"networkBufferSize"`
	ConnectTimeoutSeconds     int                     `json:"connectTimeoutSeconds"`
	DuplicateFileAction       string                  `json:"duplicateFileAction"` // "rename", "overwrite", "resume"
	ProxyMode                 string                  `json:"proxyMode"`           // "direct", "system", "manual"
	ProxyURL                  string                  `json:"proxyUrl"`
	DefaultQueueID            string                  `json:"defaultQueueId"`
	Queues                    map[string]*QueueRecord `json:"queues,omitempty"`
}

// DefaultSettings returns safe, optimized default options.
func DefaultSettings() *Settings {
	homeDir, _ := os.UserHomeDir()
	downloadDir := filepath.Join(homeDir, "Downloads")

	return &Settings{
		MaxConcurrentDownloads:    4,
		MaxConnectionsPerDownload: 8,
		GlobalSpeedLimit:          0,
		DefaultDownloadDir:        downloadDir,
		UseTargetForTemp:          true,
		TemporaryDirectory:        "",
		CategoryFolders: map[string]string{
			"Compressed": filepath.Join(downloadDir, "Compressed"),
			"Documents":  filepath.Join(downloadDir, "Documents"),
			"Videos":     filepath.Join(downloadDir, "Videos"),
			"Music":      filepath.Join(downloadDir, "Music"),
			"Programs":   filepath.Join(downloadDir, "Programs"),
			"Pictures":   filepath.Join(downloadDir, "Pictures"),
			"Other":      filepath.Join(downloadDir, "Other"),
		},
		AutoStart:             true,
		AutoFetchEnabled:      true,
		InterceptExtensions: []string{
			"zip", "rar", "7z", "tar", "gz", "bz2", "xz", "iso", "dmg",
			"exe", "msi", "bin", "apk",
			"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx",
			"mp4", "mkv", "avi", "mov", "webm", "flv",
			"mp3", "flac", "wav", "aac", "ogg", "m4a",
			"jpg", "jpeg", "png", "webp", "gif", "svg",
		},
		MinInterceptSizeBytes: 1024 * 1024, // 1 MB default
		SoundAlertOnComplete:  true,
		DarkTheme:             true,
		SelectedTheme:         "dark",
		StartMinimized:        false,
		AutoOpenCompleted:     false,
		NetworkBufferSize:     262144, // 256 KB buffer
		ConnectTimeoutSeconds: 15,
		DuplicateFileAction:   "prompt",
		ProxyMode:             "direct",
		DefaultQueueID:        "main_queue",
		Queues:                make(map[string]*QueueRecord),
	}
}

// DownloadRecord stores the serialized state of a download item.
type DownloadRecord struct {
	ID              string            `json:"id"`
	URL             string            `json:"url"`
	Filename        string            `json:"filename"`
	SavePath        string            `json:"savePath"`
	FinalPath       string            `json:"finalPath"`
	TotalSize       int64             `json:"totalSize"`
	DownloadedBytes int64             `json:"downloadedBytes"`
	Status          string            `json:"status"` // Queued, Connecting, Downloading, Paused, Completed, Error
	Category        string            `json:"category"`
	QueueID         string            `json:"queueId"`
	QueueIndex      int               `json:"queueIndex"`
	Speed           int64             `json:"speed"`
	SpeedLimit      int64             `json:"speedLimit"` // per-task speed limit in bytes/sec, 0 = unlimited
	MaxConns        int               `json:"maxConns"`   // max connections for this task
	ETA             int64             `json:"eta"`        // seconds
	Segments        []*Segment        `json:"segments"`
	SupportsRange   bool              `json:"supportsRange"`
	ConflictAction  string            `json:"conflictAction,omitempty"` // "prompt", "overwrite", "rename"
	Headers         map[string]string `json:"headers"`
	CreatedAt       time.Time         `json:"createdAt"`
	CompletedAt     *time.Time        `json:"completedAt,omitempty"`
	Error           string            `json:"error,omitempty"`
}

// Store persists download records and settings safely.
type Store struct {
	mu           sync.Mutex
	dataDir      string
	settingsPath string
	recordsPath  string
	Settings     *Settings
	Records      map[string]*DownloadRecord
	dirty        bool
}

// NewStoreWithPath initializes the data store at a custom directory path (for testing or isolation).
func NewStoreWithPath(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	s := &Store{
		dataDir:      dataDir,
		settingsPath: filepath.Join(dataDir, "settings.json"),
		recordsPath:  filepath.Join(dataDir, "downloads.json"),
		Settings:     DefaultSettings(),
		Records:      make(map[string]*DownloadRecord),
	}

	s.load()

	// Background debounced flusher for telemetry updates
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s.mu.Lock()
			if s.dirty {
				s.dirty = false
				s.mu.Unlock()
				_ = s.SaveRecords()
			} else {
				s.mu.Unlock()
			}
		}
	}()

	return s, nil
}

// NewStore initializes the data store at %APPDATA%\IFD or ./data
func NewStore() (*Store, error) {
	appData := os.Getenv("APPDATA")
	var dataDir string
	if appData != "" {
		dataDir = filepath.Join(appData, "IFD")
	} else {
		dataDir = "./data"
	}

	return NewStoreWithPath(dataDir)
}

func (s *Store) load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	homeDir, _ := os.UserHomeDir()
	userDownloads := filepath.Join(homeDir, "Downloads")

	// Load settings
	if data, err := os.ReadFile(s.settingsPath); err == nil {
		var loadedSettings Settings
		if err := json.Unmarshal(data, &loadedSettings); err == nil {
			s.Settings = &loadedSettings
		}
	}

	// Safety sanitization: guarantee safe defaults for concurrency and download directory
	if s.Settings.MaxConcurrentDownloads <= 0 {
		s.Settings.MaxConcurrentDownloads = 4
	}
	if s.Settings.MaxConnectionsPerDownload <= 0 {
		s.Settings.MaxConnectionsPerDownload = 8
	}
	if s.Settings.DefaultDownloadDir == "" || strings.Contains(s.Settings.DefaultDownloadDir, "Temp") {
		s.Settings.DefaultDownloadDir = userDownloads
	}

	// Load records
	if data, err := os.ReadFile(s.recordsPath); err == nil {
		var loadedRecords []*DownloadRecord
		if err := json.Unmarshal(data, &loadedRecords); err == nil {
			for _, rec := range loadedRecords {
				// Normalize status on restart: active downloads become paused
				if rec.Status == "Downloading" || rec.Status == "Connecting" {
					rec.Status = "Paused"
					rec.Speed = 0
				}
				s.Records[rec.ID] = rec
			}
		}
	}
}

// SaveSettings writes settings to disk atomically.
func (s *Store) SaveSettings() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(s.Settings, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := s.settingsPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, s.settingsPath)
}

// SaveRecords writes download records to disk atomically.
func (s *Store) SaveRecords() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]*DownloadRecord, 0, len(s.Records))
	for _, rec := range s.Records {
		list = append(list, rec)
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := s.recordsPath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, s.recordsPath)
}

// UpdateRecord updates or adds a record and persists to disk (debounced during streaming).
func (s *Store) UpdateRecord(rec *DownloadRecord) {
	s.mu.Lock()
	s.Records[rec.ID] = rec

	immediate := rec.Status == "Completed" || rec.Status == "Paused" || rec.Status == "Error" || rec.Status == "Queued"
	if !immediate {
		s.dirty = true
		s.mu.Unlock()
		return
	}
	s.dirty = false
	s.mu.Unlock()

	_ = s.SaveRecords()
}

// DeleteRecord deletes a record and persists.
func (s *Store) DeleteRecord(id string) {
	s.mu.Lock()
	delete(s.Records, id)
	s.mu.Unlock()
	_ = s.SaveRecords()
}
