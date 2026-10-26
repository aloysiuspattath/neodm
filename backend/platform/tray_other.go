//go:build !windows
// +build !windows

package platform

type TrayCallbacks struct {
	OnShow        func()
	OnStartAll    func()
	OnPauseAll    func()
	OnAddDownload func()
	OnSettings    func()
	OnExit        func()
}

type TrayManager struct{}

func GetGlobalTray() *TrayManager { return nil }

func NewTrayManager(callbacks TrayCallbacks) (*TrayManager, error) {
	return &TrayManager{}, nil
}

func (tm *TrayManager) UpdateTooltip(tip string) {}

func (tm *TrayManager) ShowBalloonNotification(title, text string) {}

func (tm *TrayManager) Destroy() {}

func ShowAppWindow() {}

func HideAppWindow() {}
