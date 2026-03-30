package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type GitMode int

const (
	ModeUnstaged GitMode = iota
	ModeStaged
	ModeBranch
)

func (m GitMode) String() string {
	switch m {
	case ModeUnstaged:
		return "Unstaged"
	case ModeStaged:
		return "Staged"
	case ModeBranch:
		return "Branch"
	}
	return ""
}

type FileEntry struct {
	Status    byte   // A, M, D, R, ?, etc.
	Path      string
	Additions int
	Deletions int
}

type Info struct {
	Branch       string
	RemoteBranch string

	Unstaged    []FileEntry
	Staged      []FileEntry
	BranchFiles []FileEntry

	UnstagedCount  int
	StagedCount    int
	UntrackedCount int

	BranchBase    string
	AheadCount    int
	BehindCount   int
	BranchCommits []string
}

func GetInfo(cwd string) *Info {
	info := &Info{}

	if out, err := runGit(cwd, "branch", "--show-current"); err == nil {
		info.Branch = strings.TrimSpace(out)
	}

	info.BranchBase = detectBaseBranch(cwd)
	info.RemoteBranch = info.BranchBase

	// Parse git status for staged/unstaged/untracked
	if out, err := runGit(cwd, "status", "--porcelain"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if len(line) < 3 {
				continue
			}
			staging := line[0]
			working := line[1]
			path := strings.TrimSpace(line[3:])

			if staging != ' ' && staging != '?' {
				info.Staged = append(info.Staged, FileEntry{Status: staging, Path: path})
				info.StagedCount++
			}
			if working != ' ' && working != '?' {
				info.Unstaged = append(info.Unstaged, FileEntry{Status: working, Path: path})
				info.UnstagedCount++
			}
			if staging == '?' {
				info.Unstaged = append(info.Unstaged, FileEntry{Status: '?', Path: path})
				info.UntrackedCount++
			}
		}
	}

	// Get numstat for unstaged files (additions/deletions per file)
	fillNumstat(cwd, info.Unstaged, "diff", "--numstat")
	// Get numstat for staged files
	fillNumstat(cwd, info.Staged, "diff", "--cached", "--numstat")

	// Branch diff
	if info.BranchBase != "" {
		if out, err := runGit(cwd, "diff", "--name-status", info.BranchBase+"...HEAD"); err == nil {
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if len(line) < 2 {
					continue
				}
				parts := strings.Fields(line)
				if len(parts) < 2 {
					continue
				}
				info.BranchFiles = append(info.BranchFiles, FileEntry{
					Status: parts[0][0],
					Path:   parts[len(parts)-1],
				})
			}
		}

		// Numstat for branch files
		fillNumstat(cwd, info.BranchFiles, "diff", "--numstat", info.BranchBase+"...HEAD")

		// Ahead/behind
		if out, err := runGit(cwd, "rev-list", "--left-right", "--count", info.BranchBase+"...HEAD"); err == nil {
			parts := strings.Fields(strings.TrimSpace(out))
			if len(parts) == 2 {
				fmt.Sscanf(parts[0], "%d", &info.BehindCount)
				fmt.Sscanf(parts[1], "%d", &info.AheadCount)
			}
		}

		// Commits on branch
		if out, err := runGit(cwd, "log", "--oneline", info.BranchBase+"..HEAD"); err == nil {
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					info.BranchCommits = append(info.BranchCommits, line)
				}
			}
		}
	}

	return info
}

// fillNumstat runs git diff --numstat and fills in additions/deletions for matching entries
func fillNumstat(cwd string, entries []FileEntry, args ...string) {
	if len(entries) == 0 {
		return
	}
	out, err := runGit(cwd, args...)
	if err != nil {
		return
	}
	// Build path -> index map
	pathIdx := make(map[string][]int)
	for i, e := range entries {
		pathIdx[e.Path] = append(pathIdx[e.Path], i)
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}
		add, _ := strconv.Atoi(parts[0])
		del, _ := strconv.Atoi(parts[1])
		path := parts[2]
		for _, idx := range pathIdx[path] {
			entries[idx].Additions = add
			entries[idx].Deletions = del
		}
	}
}

func detectBaseBranch(cwd string) string {
	if _, err := runGit(cwd, "rev-parse", "--verify", "origin/master"); err == nil {
		return "origin/master"
	}
	if _, err := runGit(cwd, "rev-parse", "--verify", "origin/main"); err == nil {
		return "origin/main"
	}
	return ""
}

func (info *Info) FilesForMode(mode GitMode) []FileEntry {
	switch mode {
	case ModeUnstaged:
		return info.Unstaged
	case ModeStaged:
		return info.Staged
	case ModeBranch:
		return info.BranchFiles
	}
	return nil
}

func GetFileDiff(cwd, path string, mode GitMode) string {
	switch mode {
	case ModeUnstaged:
		if out, err := runGit(cwd, "diff", "--", path); err == nil && strings.TrimSpace(out) != "" {
			return out
		}
		if content, err := readFileHead(cwd, path, 200); err == nil {
			return "new file: " + path + "\n\n" + content
		}
	case ModeStaged:
		if out, err := runGit(cwd, "diff", "--cached", "--", path); err == nil && strings.TrimSpace(out) != "" {
			return out
		}
	case ModeBranch:
		base := detectBaseBranch(cwd)
		if base != "" {
			if out, err := runGit(cwd, "diff", base+"...HEAD", "--", path); err == nil && strings.TrimSpace(out) != "" {
				return out
			}
		}
	}
	return ""
}

func readFileHead(cwd, path string, maxLines int) (string, error) {
	fullPath := filepath.Join(cwd, path)
	f, err := os.Open(fullPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lines []string
	for i := 0; i < maxLines && scanner.Scan(); i++ {
		lines = append(lines, scanner.Text())
	}
	return strings.Join(lines, "\n"), nil
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
