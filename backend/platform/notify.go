package platform

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// ShowNotification displays a native Windows 10/11 toast notification.
func ShowNotification(title string, message string) {
	go func() {
		replacer := strings.NewReplacer(`"`, "", `$`, "", "`", "")
		safeTitle := replacer.Replace(title)
		safeMsg := replacer.Replace(message)

		psScript := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
$template = [Windows.UI.Notifications.ToastTemplateType]::ToastText02
$xml = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent($template)
$textNodes = $xml.GetElementsByTagName("text")
$textNodes.Item(0).AppendChild($xml.CreateTextNode("%s")) > $null
$textNodes.Item(1).AppendChild($xml.CreateTextNode("%s")) > $null
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
$notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("IFD")
$notifier.Show($toast)
`, safeTitle, safeMsg)

		cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = cmd.Run()
	}()
}
