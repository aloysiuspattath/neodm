//go:build !windows
// +build !windows

package engine

type DriveMetric struct {
	Letter     string `json:"letter"`
	Label      string `json:"label"`
	TotalBytes int64  `json:"totalBytes"`
	FreeBytes  int64  `json:"freeBytes"`
}

func CheckFreeDiskSpace(targetPath string, requiredBytes int64) error {
	return nil
}

func GetDriveMetrics() ([]DriveMetric, error) {
	return []DriveMetric{
		{
			Letter:     "/",
			Label:      "Root Volume",
			TotalBytes: 512 * 1024 * 1024 * 1024,
			FreeBytes:  128 * 1024 * 1024 * 1024,
		},
	}, nil
}
