package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Storage handles thread-safe random-access file writing directly to predetermined offsets.
type Storage struct {
	mu             sync.Mutex
	path           string
	partPath       string
	file           *os.File
	size           int64
	AllowOverwrite bool
}

// NewStorage creates or opens a pre-allocated storage file at partPath.
// If tempDirOpt is provided and non-empty, the in-progress part file is stored in that directory.
func NewStorage(finalPath string, size int64, taskID string, tempDirOpt ...string) (*Storage, error) {
	dir := filepath.Dir(finalPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// 1. Verify destination free disk space
	if err := CheckFreeDiskSpace(dir, size); err != nil {
		return nil, err
	}

	// 2. Determine partPath location
	partDir := dir
	if len(tempDirOpt) > 0 && strings.TrimSpace(tempDirOpt[0]) != "" {
		customTemp := strings.TrimSpace(tempDirOpt[0])
		if err := os.MkdirAll(customTemp, 0755); err != nil {
			return nil, fmt.Errorf("failed to create temporary directory %s: %w", customTemp, err)
		}
		if err := CheckFreeDiskSpace(customTemp, size); err != nil {
			return nil, fmt.Errorf("temporary directory disk space error: %w", err)
		}
		partDir = customTemp
	}

	baseName := filepath.Base(finalPath)
	partPath := filepath.Join(partDir, fmt.Sprintf("%s.%s.ifd_part", baseName, taskID))

	// 3. Open or create the part file
	file, err := os.OpenFile(partPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", partPath, err)
	}

	// 4. Pre-allocate file size if known
	if size > 0 {
		currentInfo, err := file.Stat()
		if err == nil && currentInfo.Size() < size {
			if truncErr := file.Truncate(size); truncErr != nil {
				_ = file.Close()
				_ = os.Remove(partPath)
				return nil, fmt.Errorf("failed to pre-allocate %d bytes: %w", size, truncErr)
			}
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

	finalDest := s.path
	if s.AllowOverwrite {
		// In overwrite mode, remove existing destination file if present so atomic rename succeeds on Windows
		if _, err := os.Stat(finalDest); err == nil {
			_ = os.Remove(finalDest)
		}
	} else {
		// Ensure destination file name uniqueness if existing file already present
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
	}

	// 1. Fast path: Atomic rename (succeeds instantaneously when on same volume)
	var renameErr error
	for attempt := 0; attempt < 5; attempt++ {
		renameErr = os.Rename(s.partPath, finalDest)
		if renameErr == nil {
			return finalDest, nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 2. Slow path: Cross-volume copy and delete (e.g. Temp on C:, Final on D:)
	copyErr := copyAndDeleteFile(s.partPath, finalDest)
	if copyErr != nil {
		return "", fmt.Errorf("failed to finalize download to %s: %w (rename error: %v)", finalDest, copyErr, renameErr)
	}

	return finalDest, nil
}

func copyAndDeleteFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return err
	}
	defer destFile.Close()

	buf := make([]byte, 1024*1024) // 1MB buffer for fast streaming transfer
	if _, err := io.CopyBuffer(destFile, sourceFile, buf); err != nil {
		_ = os.Remove(dst)
		return err
	}

	_ = destFile.Sync()
	_ = sourceFile.Close()
	_ = destFile.Close()
	return os.Remove(src)
}

// RemovePartFile removes the unfinished .ifd_part file on download cancellation.
func (s *Storage) RemovePartFile() error {
	_ = s.Close()
	if _, err := os.Stat(s.partPath); err == nil {
		return os.Remove(s.partPath)
	}
	return nil
}
