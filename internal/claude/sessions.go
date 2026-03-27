package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Session struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	Cwd        string `json:"cwd"`
	StartedAt  int64  `json:"startedAt"`
	Kind       string `json:"kind"`
	Entrypoint string `json:"entrypoint"`
}

func (s Session) StartTime() time.Time {
	return time.UnixMilli(s.StartedAt)
}

func (s Session) Age() time.Duration {
	return time.Since(s.StartTime())
}

func (s Session) ShortID() string {
	if len(s.SessionID) >= 8 {
		return s.SessionID[:8]
	}
	return s.SessionID
}

func (s Session) ProjectDir() string {
	return strings.ReplaceAll(s.Cwd, "/", "-")
}

func (s Session) JSONLPath() string {
	homeDir, _ := os.UserHomeDir()
	projectsDir := filepath.Join(homeDir, ".claude", "projects")

	// Try exact match first
	direct := filepath.Join(projectsDir, s.ProjectDir(), s.SessionID+".jsonl")
	if _, err := os.Stat(direct); err == nil {
		return direct
	}

	// Search project dirs that start with our prefix (handles worktree suffixes)
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return direct
	}

	prefix := s.ProjectDir()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		candidate := filepath.Join(projectsDir, entry.Name(), s.SessionID+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return direct
}

func IsProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func claudeSessionsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home dir: %w", err)
	}
	return filepath.Join(homeDir, ".claude", "sessions"), nil
}

func DiscoverSessions(filterCwd string) ([]Session, error) {
	dir, err := claudeSessionsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading sessions dir: %w", err)
	}

	var sessions []Session
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}

		// Filter by cwd if specified
		if filterCwd != "" && s.Cwd != filterCwd {
			continue
		}

		// Only include sessions with a live process
		if !IsProcessAlive(s.PID) {
			continue
		}

		sessions = append(sessions, s)
	}

	return sessions, nil
}

func FormatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if hours < 24 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	days := hours / 24
	return fmt.Sprintf("%dd %dh", days, hours%24)
}
