package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

type Usage struct {
	InputTokens              int    `json:"input_tokens"`
	CacheCreationInputTokens int    `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int    `json:"cache_read_input_tokens"`
	OutputTokens             int    `json:"output_tokens"`
	Speed                    string `json:"speed,omitempty"`
	ServiceTier              string `json:"service_tier,omitempty"`
}

// ContextTokens returns the total context used in this usage snapshot
func (u Usage) ContextTokens() int {
	return u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
}

type ContentBlock struct {
	Type  string `json:"type"`
	Name  string `json:"name,omitempty"`
	Text  string `json:"text,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type Message struct {
	Model      string         `json:"model"`
	Usage      Usage          `json:"usage"`
	StopReason *string        `json:"stop_reason"`
	Content    []ContentBlock `json:"content"`
}

type UserContent struct {
	Content interface{} `json:"content"`
}

type JournalEntry struct {
	Type      string       `json:"type"`
	UUID      string       `json:"uuid,omitempty"`
	Timestamp string       `json:"timestamp,omitempty"`
	Message   *Message     `json:"message,omitempty"`
	IsMeta    *bool        `json:"isMeta,omitempty"`
	GitBranch string       `json:"gitBranch,omitempty"`
	Cwd       string       `json:"cwd,omitempty"`
	Subtype   string       `json:"subtype,omitempty"`
}

type ActivityItem struct {
	Time    time.Time
	Kind    string // "user", "assistant", "tool", "system"
	Summary string
}

type SessionData struct {
	Model      string
	Branch     string
	Turns      int
	Tokens     Usage
	LastUsage  Usage  // most recent completed message — for context %
	Speed      string // "standard", "fast", etc.
	Activities []ActivityItem
	LastUpdate time.Time
}

// ParseJSONL reads a session JSONL file and extracts aggregated data.
// It handles streaming duplicates by taking the last line per uuid.
func ParseJSONL(path string) (*SessionData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data := &SessionData{}
	// Track last usage per uuid to handle streaming duplicates
	uuidUsage := make(map[string]Usage)
	uuidModel := make(map[string]string)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large lines

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry JournalEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		switch entry.Type {
		case "assistant":
			if entry.Message == nil {
				continue
			}
			msg := entry.Message

			// Track usage per uuid (last wins for streaming)
			if entry.UUID != "" && (msg.Usage.InputTokens > 0 || msg.Usage.OutputTokens > 0) {
				uuidUsage[entry.UUID] = msg.Usage
			}

			if msg.Model != "" {
				if entry.UUID != "" {
					uuidModel[entry.UUID] = msg.Model
				}
				data.Model = msg.Model
			}

			if entry.GitBranch != "" {
				data.Branch = entry.GitBranch
			}

			// Track last completed message usage for context %
			if msg.StopReason != nil && (msg.Usage.InputTokens > 0 || msg.Usage.CacheReadInputTokens > 0) {
				data.LastUsage = msg.Usage
			}

			// Track speed/effort
			if msg.Usage.Speed != "" {
				data.Speed = msg.Usage.Speed
			}

			// Only count completed messages as activity
			if msg.StopReason != nil {
				ts := parseTimestamp(entry.Timestamp)
				// Extract tool uses
				for _, block := range msg.Content {
					if block.Type == "tool_use" {
						data.Activities = append(data.Activities, ActivityItem{
							Time:    ts,
							Kind:    "tool",
							Summary: block.Name,
						})
					}
				}
				// Count text responses
				for _, block := range msg.Content {
					if block.Type == "text" && len(block.Text) > 0 {
						summary := block.Text
						if len(summary) > 60 {
							summary = summary[:60] + "…"
						}
						data.Activities = append(data.Activities, ActivityItem{
							Time:    ts,
							Kind:    "assistant",
							Summary: summary,
						})
						break // only first text block
					}
				}
			}

		case "user":
			if entry.IsMeta != nil && *entry.IsMeta {
				continue
			}
			data.Turns++
			ts := parseTimestamp(entry.Timestamp)
			if !ts.IsZero() {
				data.Activities = append(data.Activities, ActivityItem{
					Time:    ts,
					Kind:    "user",
					Summary: "user message",
				})
			}
		}

		if ts := parseTimestamp(entry.Timestamp); !ts.IsZero() && ts.After(data.LastUpdate) {
			data.LastUpdate = ts
		}
	}

	// Sum up deduplicated token usage
	for _, u := range uuidUsage {
		data.Tokens.InputTokens += u.InputTokens
		data.Tokens.CacheCreationInputTokens += u.CacheCreationInputTokens
		data.Tokens.CacheReadInputTokens += u.CacheReadInputTokens
		data.Tokens.OutputTokens += u.OutputTokens
	}

	// Pick the most common model
	if data.Model == "" {
		for _, m := range uuidModel {
			data.Model = m
			break
		}
	}

	return data, nil
}

// ParseJSONLFrom reads a JSONL file starting from a byte offset, for incremental updates.
func ParseJSONLFrom(path string, offset int64) (*SessionData, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return nil, offset, err
		}
	}

	data := &SessionData{}
	uuidUsage := make(map[string]Usage)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry JournalEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		if entry.Type == "assistant" && entry.Message != nil {
			msg := entry.Message
			if entry.UUID != "" && (msg.Usage.InputTokens > 0 || msg.Usage.OutputTokens > 0) {
				uuidUsage[entry.UUID] = msg.Usage
			}
			if msg.Model != "" {
				data.Model = msg.Model
			}
			if entry.GitBranch != "" {
				data.Branch = entry.GitBranch
			}
			// Track last completed message for context %
			if msg.StopReason != nil && (msg.Usage.InputTokens > 0 || msg.Usage.CacheReadInputTokens > 0) {
				data.LastUsage = msg.Usage
			}
			if msg.Usage.Speed != "" {
				data.Speed = msg.Usage.Speed
			}
		}

		if entry.Type == "user" {
			if entry.IsMeta != nil && *entry.IsMeta {
				continue
			}
			data.Turns++
		}

		if ts := parseTimestamp(entry.Timestamp); !ts.IsZero() && ts.After(data.LastUpdate) {
			data.LastUpdate = ts
		}
	}

	for _, u := range uuidUsage {
		data.Tokens.InputTokens += u.InputTokens
		data.Tokens.CacheCreationInputTokens += u.CacheCreationInputTokens
		data.Tokens.CacheReadInputTokens += u.CacheReadInputTokens
		data.Tokens.OutputTokens += u.OutputTokens
	}

	newOffset, _ := f.Seek(0, 2) // seek to end
	return data, newOffset, nil
}

func parseTimestamp(ts string) time.Time {
	if ts == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05.000Z", ts)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}
