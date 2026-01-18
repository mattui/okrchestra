package notify

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Notifier sends system notifications.
type Notifier struct {
	Enabled bool
}

// Send sends a system notification.
// On macOS, uses osascript to display notifications.
// On other platforms, this is a no-op.
func (n *Notifier) Send(title, message string) error {
	if !n.Enabled {
		return nil
	}

	if runtime.GOOS != "darwin" {
		// Only macOS supported for now
		return nil
	}

	return sendMacOSNotification(title, message)
}

// sendMacOSNotification uses osascript to display a notification.
func sendMacOSNotification(title, message string) error {
	// Escape quotes in title and message
	title = strings.ReplaceAll(title, `"`, `\"`)
	message = strings.ReplaceAll(message, `"`, `\"`)

	script := fmt.Sprintf(`display notification "%s" with title "%s"`, message, title)
	cmd := exec.Command("osascript", "-e", script)
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	
	return nil
}

// FormatPlanComplete formats a plan completion notification message.
func FormatPlanComplete(planID string, itemsTotal, itemsSucceeded, itemsFailed int, krID string) (title, message string) {
	if itemsFailed > 0 {
		title = "⚠️ OKRchestra Plan Failed"
		message = fmt.Sprintf("%s: %d/%d items failed", krID, itemsFailed, itemsTotal)
	} else {
		title = "✅ OKRchestra Plan Complete"
		message = fmt.Sprintf("%s: %d/%d items succeeded", krID, itemsSucceeded, itemsTotal)
	}
	return title, message
}

// FormatKRAchieved formats a KR achievement notification message.
func FormatKRAchieved(krID, description string, current, target float64) (title, message string) {
	title = "🎉 OKRchestra KR Achieved"
	message = fmt.Sprintf("%s: %s (%.0f/%.0f)", krID, description, current, target)
	return title, message
}

// FormatKRStatusChange formats a KR status change notification message.
func FormatKRStatusChange(krID, description, oldStatus, newStatus string, current, target float64) (title, message string) {
	switch newStatus {
	case "achieved":
		return FormatKRAchieved(krID, description, current, target)
	case "in_progress":
		title = "🚀 OKRchestra KR In Progress"
		message = fmt.Sprintf("%s: %s (%.0f/%.0f)", krID, description, current, target)
	default:
		title = "📊 OKRchestra KR Status Update"
		message = fmt.Sprintf("%s: %s → %s", krID, oldStatus, newStatus)
	}
	return title, message
}
