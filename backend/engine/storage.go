package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Storage handles thread-safe random-access file writing directly to predetermined offsets.
type Storage struct {
	mu       sync.Mutex
	path     string
	partPath string
	file     *os.File
	size     int64
}

// NewStorage creates or opens a pre-allocated storage file at partPath.
func NewStorage(finalPath string, size int64, taskID string) (*Storage, error) {
	dir := filepath.Dir(finalPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	partPath := fmt.Sprintf("%s.%s.ifd_part", finalPath, taskID)

	// Open or create the part file
	file, err := os.OpenFile(partPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", partPath, err)
	}

	// Pre-allocate file size if known
	if size > 0 {
		currentInfo, err := file.Stat()
		if err == nil && currentInfo.Size() < size {
			_ = file.Truncate(size)
		}
	}

	return &Storage{
		path:     finalPath,
		partPath: partPath,
		file:     file,
		size:     size,
	}, nil
}

// WriteAt writes data to the specified byte offset.
func (s *Storage) WriteAt(p []byte, off int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return 0, fmt.Errorf("storage file is closed")
	}
	return s.file.WriteAt(p, off)
}

// Sync flushes data to disk.
func (s *Storage) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	return s.file.Sync()
}

// Close closes the file handle.
func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

// Finalize closes the part file and atomically renames it to the final file name.
func (s *Storage) Finalize() (string, error) {
	if err := s.Close(); err != nil {
		return "", err
	}

	// Ensure destination file name uniqueness if existing file already present
	finalDest := s.path
	ext := filepath.Ext(finalDest)
	baseWithoutExt := finalDest[:len(finalDest)-len(ext)]
	counter := 1

	for {
		if _, err := os.Stat(finalDest); os.IsNotExist(err) {
			break
		}
		// If existing file is identical or already there, find next available filename
		finalDest = fmt.Sprintf("%s (%d)%s", baseWithoutExt, counter, ext)
		counter++
	}

	var renameErr error
	for attempt := 0; attempt < 5; attempt++ {
		renameErr = os.Rename(s.partPath, finalDest)
		if renameErr == nil {
			return finalDest, nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return "", fmt.Errorf("failed to finalize download to %s: %w", finalDest, renameErr)
}

// RemovePartFile removes the unfinished .ifd_part file on download cancellation.
func (s *Storage) RemovePartFile() error {
	_ = s.Close()
	if _, err := os.Stat(s.partPath); err == nil {
		return os.Remove(s.partPath)
	}
	return nil
}
