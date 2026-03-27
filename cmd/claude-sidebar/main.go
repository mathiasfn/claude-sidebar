package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mathias/claude-sidebar/internal/claude"
	gitpkg "github.com/mathias/claude-sidebar/internal/git"
	"github.com/mathias/claude-sidebar/internal/tokens"
	"github.com/mathias/claude-sidebar/internal/tui"
)

func main() {
	cwd := flag.String("cwd", "", "Directory to monitor (default: current directory)")
	jsonMode := flag.Bool("json", false, "Dump session info as JSON and exit")
	allSessions := flag.Bool("all", false, "Show all active sessions (any cwd)")
	flag.Parse()

	dir := *cwd
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting working directory: %v\n", err)
			os.Exit(1)
		}
	}

	if *jsonMode {
		dumpJSON(dir, *allSessions)
		return
	}

	if *allSessions {
		dir = ""
	}

	model := tui.NewModel(dir)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

type jsonSession struct {
	SessionID string  `json:"session_id"`
	PID       int     `json:"pid"`
	Cwd       string  `json:"cwd"`
	Model     string  `json:"model"`
	Age       string  `json:"age"`
	Turns     int     `json:"turns"`
	Input     int     `json:"input_tokens"`
	Output    int     `json:"output_tokens"`
	CacheRead int     `json:"cache_read_tokens"`
	CacheWrite int    `json:"cache_write_tokens"`
	Cost      float64 `json:"estimated_cost"`
}

type jsonOutput struct {
	Cwd      string        `json:"cwd"`
	Branch   string        `json:"branch"`
	Sessions []jsonSession `json:"sessions"`
	Total    struct {
		Input      int     `json:"input_tokens"`
		Output     int     `json:"output_tokens"`
		CacheRead  int     `json:"cache_read_tokens"`
		CacheWrite int     `json:"cache_write_tokens"`
		Cost       float64 `json:"estimated_cost"`
	} `json:"total"`
}

func dumpJSON(dir string, all bool) {
	filterCwd := dir
	if all {
		filterCwd = ""
	}

	sessions, err := claude.DiscoverSessions(filterCwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering sessions: %v\n", err)
		os.Exit(1)
	}

	gitInfo := gitpkg.GetInfo(dir)

	out := jsonOutput{
		Cwd:    dir,
		Branch: gitInfo.Branch,
	}

	var totalUsage claude.Usage
	for _, sess := range sessions {
		data, err := claude.ParseJSONL(sess.JSONLPath())
		if err != nil {
			data = &claude.SessionData{}
		}

		cost := tokens.EstimateCost(data.Tokens, data.Model)
		out.Sessions = append(out.Sessions, jsonSession{
			SessionID:  sess.SessionID,
			PID:        sess.PID,
			Cwd:        sess.Cwd,
			Model:      data.Model,
			Age:        claude.FormatAge(sess.Age()),
			Turns:      data.Turns,
			Input:      data.Tokens.InputTokens,
			Output:     data.Tokens.OutputTokens,
			CacheRead:  data.Tokens.CacheReadInputTokens,
			CacheWrite: data.Tokens.CacheCreationInputTokens,
			Cost:       cost,
		})

		totalUsage.InputTokens += data.Tokens.InputTokens
		totalUsage.OutputTokens += data.Tokens.OutputTokens
		totalUsage.CacheReadInputTokens += data.Tokens.CacheReadInputTokens
		totalUsage.CacheCreationInputTokens += data.Tokens.CacheCreationInputTokens
	}

	out.Total.Input = totalUsage.InputTokens
	out.Total.Output = totalUsage.OutputTokens
	out.Total.CacheRead = totalUsage.CacheReadInputTokens
	out.Total.CacheWrite = totalUsage.CacheCreationInputTokens
	out.Total.Cost = tokens.EstimateCost(totalUsage, "claude-opus-4-6")

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}
