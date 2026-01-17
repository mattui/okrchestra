package daemon

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"okrchestra/internal/workspace"
)

// WorkspaceHash generates a stable short hash from the workspace root path.
func WorkspaceHash(wsRoot string) string {
	h := sha256.Sum256([]byte(wsRoot))
	return fmt.Sprintf("%x", h[:4]) // 8 hex chars
}

// PlistLabel returns the LaunchAgent label for a workspace.
func PlistLabel(wsRoot string) string {
	return fmt.Sprintf("ai.okrchestra.%s", WorkspaceHash(wsRoot))
}

// PlistPath returns the full path to the plist file for a workspace.
func PlistPath(wsRoot string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	label := PlistLabel(wsRoot)
	return filepath.Join(homeDir, "Library", "LaunchAgents", label+".plist"), nil
}

// GeneratePlist creates a plist XML string for the okrchestra daemon.
func GeneratePlist(ws *workspace.Workspace, binaryPath string) (string, error) {
	if ws == nil {
		return "", fmt.Errorf("workspace is nil")
	}

	// Ensure binary path is absolute
	absBinaryPath, err := filepath.Abs(binaryPath)
	if err != nil {
		return "", fmt.Errorf("resolve binary path: %w", err)
	}

	label := PlistLabel(ws.Root)
	logPath := filepath.Join(ws.LogDir, "okrchestra.log")

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>daemon</string>
		<string>run</string>
		<string>--workspace</string>
		<string>%s</string>
	</array>
	<key>StandardOutPath</key>
	<string>%s</string>
	<key>StandardErrorPath</key>
	<string>%s</string>
	<key>KeepAlive</key>
	<true/>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`, label, absBinaryPath, ws.Root, logPath, logPath)

	return plist, nil
}

// Install writes the LaunchAgent plist for the workspace.
func Install(ws *workspace.Workspace, binaryPath string) error {
	if ws == nil {
		return fmt.Errorf("workspace is nil")
	}

	// Ensure log directory exists
	if err := os.MkdirAll(ws.LogDir, 0o755); err != nil {
		return fmt.Errorf("ensure log dir: %w", err)
	}

	// Generate plist
	plistContent, err := GeneratePlist(ws, binaryPath)
	if err != nil {
		return fmt.Errorf("generate plist: %w", err)
	}

	// Get plist path
	plistPath, err := PlistPath(ws.Root)
	if err != nil {
		return fmt.Errorf("resolve plist path: %w", err)
	}

	// Ensure LaunchAgents directory exists
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return fmt.Errorf("ensure LaunchAgents dir: %w", err)
	}

	// Write plist file
	if err := os.WriteFile(plistPath, []byte(plistContent), 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	return nil
}

// Uninstall removes the LaunchAgent plist for the workspace.
func Uninstall(ws *workspace.Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace is nil")
	}

	plistPath, err := PlistPath(ws.Root)
	if err != nil {
		return fmt.Errorf("resolve plist path: %w", err)
	}

	// Check if plist exists
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return fmt.Errorf("plist not found: %s", plistPath)
	}

	// Remove plist file
	if err := os.Remove(plistPath); err != nil {
		return fmt.Errorf("remove plist: %w", err)
	}

	return nil
}

// Start loads the LaunchAgent using launchctl.
func Start(ws *workspace.Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace is nil")
	}

	plistPath, err := PlistPath(ws.Root)
	if err != nil {
		return fmt.Errorf("resolve plist path: %w", err)
	}

	// Check if plist exists
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return fmt.Errorf("plist not found: %s (run 'okrchestra daemon install' first)", plistPath)
	}

	// Load the LaunchAgent
	cmd := exec.Command("launchctl", "load", plistPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl load failed: %w\nOutput: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

// Stop unloads the LaunchAgent using launchctl.
func Stop(ws *workspace.Workspace) error {
	if ws == nil {
		return fmt.Errorf("workspace is nil")
	}

	plistPath, err := PlistPath(ws.Root)
	if err != nil {
		return fmt.Errorf("resolve plist path: %w", err)
	}

	// Unload the LaunchAgent
	cmd := exec.Command("launchctl", "unload", plistPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// launchctl unload may fail if not loaded - that's okay
		outputStr := strings.TrimSpace(string(output))
		if !strings.Contains(outputStr, "Could not find specified service") {
			return fmt.Errorf("launchctl unload failed: %w\nOutput: %s", err, outputStr)
		}
	}

	return nil
}

// GetLogPath returns the path to the daemon log file.
func GetLogPath(ws *workspace.Workspace) string {
	if ws == nil {
		return ""
	}
	return filepath.Join(ws.LogDir, "okrchestra.log")
}

// IsRunning checks if the daemon is currently running for this workspace.
func IsRunning(ws *workspace.Workspace) (bool, error) {
	if ws == nil {
		return false, fmt.Errorf("workspace is nil")
	}

	label := PlistLabel(ws.Root)
	cmd := exec.Command("launchctl", "list")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("launchctl list failed: %w", err)
	}

	return strings.Contains(string(output), label), nil
}
