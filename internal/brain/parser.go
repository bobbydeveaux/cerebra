package brain

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/store"
)

// hourBucket truncates an ISO timestamp to its hour bucket key, e.g. "2026-04-28T14".
func hourBucket(ts string) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		// Try alternate format from some JSONL files
		t, err = time.Parse("2006-01-02T15:04:05.000Z", ts)
		if err != nil {
			return ""
		}
	}
	return t.Format("2006-01-02T15")
}

// jsonlLine is the minimal envelope we decode from each JSONL line.
type jsonlLine struct {
	Type       string          `json:"type"`
	Timestamp  string          `json:"timestamp"`
	SessionID  string          `json:"sessionId"`
	CWD        string          `json:"cwd"`
	GitBranch  string          `json:"gitBranch"`
	Entrypoint string          `json:"entrypoint"`
	Version    string          `json:"version"`
	Slug       string          `json:"slug"`
	Message    *messagePayload `json:"message,omitempty"`
}

type messagePayload struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Model   string          `json:"model"`
	Usage   *usagePayload   `json:"usage,omitempty"`
}

type usagePayload struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// ParseSessionFile parses a Claude Code JSONL conversation file.
// If offset > 0, it seeks to that position for incremental reading.
// Returns the parsed brain, per-hour activity buckets, the new byte offset, and any error.
func ParseSessionFile(path string, offset int64) (*store.Brain, map[string]*store.HourlyActivity, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, 0, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, nil, 0, err
		}
	}

	projectKey := filepath.Base(filepath.Dir(path))
	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	b := &store.Brain{
		BrainID:     sessionID,
		ProjectKey:  projectKey,
		SessionFile: path,
		Status:      StatusActive,
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // 4MB max line

	activity := make(map[string]*store.HourlyActivity)

	var (
		msgCount   int
		tokens     int
		firstMsg   string
		lastTs     string
		firstTs    string
		model      string
		cwd        string
		branch     string
		agentType  string
		version    string
		slug       string
	)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry jsonlLine
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip malformed lines
		}

		// Track timestamps
		if entry.Timestamp != "" {
			lastTs = entry.Timestamp
			if firstTs == "" {
				firstTs = entry.Timestamp
			}
		}

		// Extract metadata from any message that has it
		if entry.CWD != "" && cwd == "" {
			cwd = entry.CWD
		}
		if entry.GitBranch != "" {
			branch = entry.GitBranch
		}
		if entry.Entrypoint != "" && agentType == "" {
			agentType = entry.Entrypoint
		}
		if entry.Version != "" {
			version = entry.Version
		}
		if entry.Slug != "" {
			slug = entry.Slug
		}

		// Resolve hour bucket for activity tracking
		hb := hourBucket(entry.Timestamp)
		if hb != "" {
			if activity[hb] == nil {
				activity[hb] = &store.HourlyActivity{
					BrainID:    sessionID,
					Hour:       hb,
					ProjectKey: projectKey,
				}
			}
		}

		switch entry.Type {
		case "user":
			msgCount++
			if hb != "" {
				activity[hb].UserMsgs++
			}
			if firstMsg == "" && entry.Message != nil {
				firstMsg = extractTextContent(entry.Message.Content)
			}

		case "assistant":
			msgCount++
			if hb != "" {
				activity[hb].AsstMsgs++
			}
			if entry.Message != nil {
				if entry.Message.Model != "" {
					model = entry.Message.Model
				}
				if entry.Message.Usage != nil {
					u := entry.Message.Usage
					lineTokens := u.InputTokens + u.OutputTokens
					tokens += lineTokens
					if hb != "" {
						activity[hb].Tokens += lineTokens
					}
				}
			}

		case "tool_use", "tool_result":
			if hb != "" {
				activity[hb].ToolUses++
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, 0, err
	}

	// Get final file position
	newOffset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, nil, 0, err
	}

	b.ProjectPath = cwd
	b.AgentType = agentType
	b.Model = model
	b.GitBranch = branch
	b.MessageCount = msgCount
	b.FirstMessageAt = firstTs
	b.LastMessageAt = lastTs
	b.Summary = truncate(firstMsg, 200)
	b.TokenUsage = tokens
	b.LastOffset = newOffset
	b.Slug = slug
	b.Version = version

	return b, activity, newOffset, nil
}

// MergeIncremental merges incremental parse results into an existing brain.
func MergeIncremental(existing *store.Brain, delta *store.Brain) {
	existing.MessageCount += delta.MessageCount
	existing.TokenUsage += delta.TokenUsage
	existing.LastOffset = delta.LastOffset

	if delta.LastMessageAt != "" {
		existing.LastMessageAt = delta.LastMessageAt
	}
	if delta.Model != "" {
		existing.Model = delta.Model
	}
	if delta.GitBranch != "" {
		existing.GitBranch = delta.GitBranch
	}
	if delta.Version != "" {
		existing.Version = delta.Version
	}
	if delta.Slug != "" {
		existing.Slug = delta.Slug
	}
	if existing.Summary == "" && delta.Summary != "" {
		existing.Summary = delta.Summary
	}
	if existing.ProjectPath == "" && delta.ProjectPath != "" {
		existing.ProjectPath = delta.ProjectPath
	}
	if existing.AgentType == "" && delta.AgentType != "" {
		existing.AgentType = delta.AgentType
	}
	existing.Status = StatusActive
}

// extractTextContent extracts plain text from a message content field.
// Content can be a JSON string or an array of content blocks.
func extractTextContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// Try as string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Try as array of content blocks
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		for _, block := range blocks {
			if block.Type == "text" && block.Text != "" {
				return block.Text
			}
		}
	}

	return ""
}

func truncate(s string, maxLen int) string {
	// Strip newlines for summary
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
