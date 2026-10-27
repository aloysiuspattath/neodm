//go:build !windows
// +build !windows

package engine

type DriveMetric struct {
	Mount       string  `json:"mount"`
	Letter      string  `json:"letter"`
	Label       string  `json:"label"`
	FS          string  `json:"fs"`
	TotalBytes  int64   `json:"total_bytes"`
	FreeBytes   int64   `json:"free_bytes"`
	TotalBytesC int64   `json:"totalBytes"`
	FreeBytesC  int64   `json:"freeBytes"`
	TotalStr    string  `json:"total_str"`
	FreeStr     string  `json:"free_str"`
	UsedPercent float64 `json:"used_percent"`
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
