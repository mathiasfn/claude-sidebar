package git

import (
	"fmt"
	"os/exec"
	"strings"
)

type Info struct {
	Branch     string
	DiffStat   string
	Status     []StatusEntry
	RecentLogs []string
}

type StatusEntry struct {
	Staging  byte
	Working  byte
	Path     string
}

func (e StatusEntry) Display() string {
	return string(e.Staging) + string(e.Working) + " " + e.Path
}

func GetInfo(cwd string) *Info {
	info := &Info{}

	// Branch
	if out, err := runGit(cwd, "branch", "--show-current"); err == nil {
		info.Branch = strings.TrimSpace(out)
	}

	// Status
	if out, err := runGit(cwd, "status", "--porcelain"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if len(line) < 3 {
				continue
			}
			info.Status = append(info.Status, StatusEntry{
				Staging: line[0],
				Working: line[1],
				Path:    strings.TrimSpace(line[3:]),
			})
		}
	}

	// Diff stat
	if out, err := runGit(cwd, "diff", "--stat"); err == nil {
		info.DiffStat = strings.TrimSpace(out)
	}

	// Recent log
	if out, err := runGit(cwd, "log", "--oneline", "-5"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				info.RecentLogs = append(info.RecentLogs, line)
			}
		}
	}

	return info
}

// GetFileDiff returns the diff for a specific file.
// It tries working tree diff first, then staged diff.
func GetFileDiff(cwd, path string) string {
	// Try unstaged diff first
	if out, err := runGit(cwd, "diff", "--", path); err == nil && strings.TrimSpace(out) != "" {
		return out
	}
	// Try staged diff
	if out, err := runGit(cwd, "diff", "--cached", "--", path); err == nil && strings.TrimSpace(out) != "" {
		return out
	}
	// For untracked files, show the file content
	if out, err := runGit(cwd, "show", ":"+path); err != nil {
		// Truly untracked — read from disk
		if content, err2 := readFileHead(cwd, path, 200); err2 == nil {
			return "new file: " + path + "\n\n" + content
		}
	} else {
		_ = out
	}
	return ""
}

func readFileHead(cwd, path string, maxLines int) (string, error) {
	fullPath := cwd + "/" + path
	cmd := exec.Command("head", "-n", fmt.Sprintf("%d", maxLines), fullPath)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func runGit(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
