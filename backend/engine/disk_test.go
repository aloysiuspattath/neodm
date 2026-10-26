package engine

import (
	"os"
	"testing"
)

func TestDiskSpaceChecks(t *testing.T) {
	tmpDir := t.TempDir()

	// Should pass for modest requirement (1 KB)
	if err := CheckFreeDiskSpace(tmpDir, 1024); err != nil {
		t.Fatalf("CheckFreeDiskSpace failed on TempDir for 1KB: %v", err)
	}

	// Should fail for impossibly huge requirement (10 Petabytes)
	hugeSize := int64(10 * 1024 * 1024 * 1024 * 1024 * 1024)
	if err := CheckFreeDiskSpace(tmpDir, hugeSize); err == nil {
		t.Fatalf("Expected CheckFreeDiskSpace to fail for 10 Petabytes, but got nil")
	}

	// Drive metrics enumeration
	metrics, err := GetDriveMetrics()
	if err != nil {
		t.Fatalf("GetDriveMetrics failed: %v", err)
	}
	if len(metrics) == 0 {
		t.Fatalf("Expected at least 1 active drive metric")
	}
	found := false
	for _, m := range metrics {
		if m.TotalBytes > 0 && m.FreeBytes > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected valid positive disk space in drive metrics")
	}
}

func TestStorageWithCustomTempDirectory(t *testing.T) {
	tempBase := t.TempDir()
	customTemp := tempBase + "_temp"
	targetDir := tempBase + "_target"

	_ = os.MkdirAll(customTemp, 0755)
	_ = os.MkdirAll(targetDir, 0755)

	targetFile := targetDir + "/downloaded_file.bin"
	storage, err := NewStorage(targetFile, 4096, "task_temp_1", customTemp)
	if err != nil {
		t.Fatalf("NewStorage with custom temp dir failed: %v", err)
	}

	// Part file should be inside customTemp
	if _, err := os.Stat(storage.partPath); os.IsNotExist(err) {
		t.Fatalf("Part file was not created in custom temp dir: %s", storage.partPath)
	}

	// Write data
	data := []byte("hello world with custom temp dir storage")
	if _, err := storage.WriteAt(data, 0); err != nil {
		t.Fatalf("WriteAt failed: %v", err)
	}

	// Finalize
	finalDest, err := storage.Finalize()
	if err != nil {
		t.Fatalf("Finalize failed: %v", err)
	}

	// Final file must exist in targetDir
	if _, err := os.Stat(finalDest); os.IsNotExist(err) {
		t.Fatalf("Final file not found at %s", finalDest)
	}

	// Part file in custom temp must be gone
	if _, err := os.Stat(storage.partPath); !os.IsNotExist(err) {
		t.Fatalf("Part file was not cleaned up after finalize: %s", storage.partPath)
	}
}
