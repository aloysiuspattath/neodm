//go:build windows
// +build windows

package engine

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32Dll            = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceExW = kernel32Dll.NewProc("GetDiskFreeSpaceExW")
	procGetLogicalDrives    = kernel32Dll.NewProc("GetLogicalDrives")
	procGetDriveTypeW       = kernel32Dll.NewProc("GetDriveTypeW")
	procGetVolumeInfoW      = kernel32Dll.NewProc("GetVolumeInformationW")
)

const (
	DRIVE_REMOVABLE = 2
	DRIVE_FIXED     = 3
	DRIVE_REMOTE    = 4
	DRIVE_CDROM     = 5
	DRIVE_RAMDISK   = 6
)

// DriveMetric represents storage capacity and free space on a host storage volume.
type DriveMetric struct {
	Mount       string  `json:"mount"`       // e.g. "C:"
	Letter      string  `json:"letter"`      // e.g. "C:" (backward compatibility)
	Label       string  `json:"label"`       // e.g. "Windows (OS)" or "Data"
	FS          string  `json:"fs"`          // e.g. "NTFS"
	TotalBytes  int64   `json:"total_bytes"` // total capacity in bytes
	FreeBytes   int64   `json:"free_bytes"`  // available bytes
	TotalBytesC int64   `json:"totalBytes"`  // camelCase backward compatibility
	FreeBytesC  int64   `json:"freeBytes"`   // camelCase backward compatibility
	TotalStr    string  `json:"total_str"`   // formatted string e.g. "476.1 GB"
	FreeStr     string  `json:"free_str"`    // formatted string e.g. "214.5 GB"
	UsedPercent float64 `json:"used_percent"` // 0.0 - 100.0 percentage
}

// CheckFreeDiskSpace verifies that the volume hosting targetPath has at least requiredBytes free.
func CheckFreeDiskSpace(targetPath string, requiredBytes int64) error {
	if requiredBytes <= 0 {
		return nil
	}

	absPath, err := filepath.Abs(targetPath)
	if err != nil {
		absPath = targetPath
	}
	volumePath := filepath.VolumeName(absPath) + `\`
	volumePtr, err := syscall.UTF16PtrFromString(volumePath)
	if err != nil {
		return err
	}

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes int64

	r, _, callErr := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(volumePtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalNumberOfBytes)),
		uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
	)
	if r == 0 {
		return fmt.Errorf("failed to query disk space for %s: %v", volumePath, callErr)
	}

	// 50 MB safety margin for OS file system metadata operations
	const safetyMargin = int64(50 * 1024 * 1024)
	if freeBytesAvailable < (requiredBytes + safetyMargin) {
		return fmt.Errorf("insufficient disk space on drive %s: %s available, %s required",
			volumePath,
			formatBytesDisk(freeBytesAvailable),
			formatBytesDisk(requiredBytes),
		)
	}

	return nil
}

func formatBytesDisk(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// GetDriveMetrics discovers all active fixed and removable drives and returns space statistics.
func GetDriveMetrics() ([]DriveMetric, error) {
	r, _, _ := procGetLogicalDrives.Call()
	bitmask := uint32(r)
	if bitmask == 0 {
		return nil, fmt.Errorf("failed to enumerate logical drives")
	}

	var metrics []DriveMetric

	for i := 0; i < 26; i++ {
		if (bitmask & (1 << i)) != 0 {
			letter := string(rune('A'+i)) + ":"
			rootPath := letter + `\`
			rootPtr, err := syscall.UTF16PtrFromString(rootPath)
			if err != nil {
				continue
			}

			driveType, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(rootPtr)))
			if driveType != DRIVE_FIXED && driveType != DRIVE_REMOVABLE && driveType != DRIVE_RAMDISK {
				continue
			}

			var freeBytes, totalBytes, totalFree int64
			rSpace, _, _ := procGetDiskFreeSpaceExW.Call(
				uintptr(unsafe.Pointer(rootPtr)),
				uintptr(unsafe.Pointer(&freeBytes)),
				uintptr(unsafe.Pointer(&totalBytes)),
				uintptr(unsafe.Pointer(&totalFree)),
			)
			if rSpace == 0 || totalBytes == 0 {
				continue
			}

			// Retrieve Volume Label & Filesystem Type
			var volNameBuf [256]uint16
			var fsNameBuf [256]uint16
			rVol, _, _ := procGetVolumeInfoW.Call(
				uintptr(unsafe.Pointer(rootPtr)),
				uintptr(unsafe.Pointer(&volNameBuf[0])),
				uintptr(len(volNameBuf)),
				0, 0, 0,
				uintptr(unsafe.Pointer(&fsNameBuf[0])),
				uintptr(len(fsNameBuf)),
			)
			label := ""
			fsType := "NTFS"
			if rVol != 0 {
				label = syscall.UTF16ToString(volNameBuf[:])
				if fsStr := syscall.UTF16ToString(fsNameBuf[:]); fsStr != "" {
					fsType = fsStr
				}
			}
			if label == "" {
				if letter == "C:" {
					label = "Local Disk (OS)"
				} else {
					label = "Local Storage (" + letter + ")"
				}
			}

			usedBytes := totalBytes - freeBytes
			var usedPercent float64
			if totalBytes > 0 {
				usedPercent = float64(usedBytes) / float64(totalBytes) * 100.0
			}

			metrics = append(metrics, DriveMetric{
				Mount:       letter,
				Letter:      letter,
				Label:       label,
				FS:          fsType,
				TotalBytes:  totalBytes,
				FreeBytes:   freeBytes,
				TotalBytesC: totalBytes,
				FreeBytesC:  freeBytes,
				TotalStr:    formatBytesDisk(totalBytes),
				FreeStr:     formatBytesDisk(freeBytes),
				UsedPercent: usedPercent,
			})
		}
	}

	return metrics, nil
}
