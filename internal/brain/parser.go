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

// agentToolUseInput captures the fields we care about from an Agent tool_use's input.
type agentToolUseInput struct {
	SubagentType string `json:"subagent_type"`
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
}

// contentBlock represents one element of a message.content array (assistant or user role).
type contentBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id,omitempty"`          // tool_use id
	Name      string          `json:"name,omitempty"`        // tool_use name
	Input     json.RawMessage `json:"input,omitempty"`       // tool_use input
	ToolUseID string          `json:"tool_use_id,omitempty"` // tool_result back-reference
	Content   json.RawMessage `json:"content,omitempty"`     // tool_result content (string or blocks)
	Text      string          `json:"text,omitempty"`        // text block
}

// parseState bundles the per-session running tallies so that the parser's
// dispatch helpers can mutate them without an unwieldy argument list.
type parseState struct {
	msgCount  int
	tokens    int
	firstMsg  string
	firstTs   string
	lastTs    string
	model     string
	cwd       string
	branch    string
	agentType string
	version   string
	slug      string
}

// ParseSessionFile parses a Claude Code JSONL conversation file.
// If offset > 0, it seeks to that position for incremental reading.
// Returns the parsed brain, per-hour activity buckets, captured agent messages,
// the new byte offset, and any error.
func ParseSessionFile(path string, offset int64) (*store.Brain, map[string]*store.HourlyActivity, []store.AgentMessage, int64, error) {
	f, err := openSessionFile(path, offset)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	defer f.Close()

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
	// Captured agent (subagent) invocations keyed by tool_use_id. Tool_use and
	// tool_result may appear on different lines (potentially across incremental
	// parses), so we accumulate and emit at the end.
	agentMsgs := make(map[string]*store.AgentMessage)

	state := &parseState{}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry jsonlLine
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip malformed lines
		}

		recordTimestamps(state, entry.Timestamp)
		updateSessionHeader(state, entry)

		hb := hourBucket(entry.Timestamp)
		ensureHourBucket(activity, sessionID, projectKey, hb)

		dispatchEntry(entry, hb, activity, agentMsgs, state, sessionID, projectKey)
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, nil, 0, err
	}

	// Get final file position
	newOffset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, nil, nil, 0, err
	}

	agentMessages := assembleBrain(b, state, agentMsgs, newOffset)

	return b, activity, agentMessages, newOffset, nil
}

// openSessionFile opens path for reading and optionally seeks to offset.
// Callers are responsible for closing the returned file.
func openSessionFile(path string, offset int64) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

// recordTimestamps tracks the first and last timestamps seen on the stream.
func recordTimestamps(state *parseState, ts string) {
	if ts == "" {
		return
	}
	state.lastTs = ts
	if state.firstTs == "" {
		state.firstTs = ts
	}
}

// updateSessionHeader copies metadata from a JSONL line into the running state.
// First-write-wins for cwd/agentType (they should never change mid-session);
// last-write-wins for branch/version/slug (they can change as Claude Code updates).
func updateSessionHeader(state *parseState, entry jsonlLine) {
	if entry.CWD != "" && state.cwd == "" {
		state.cwd = entry.CWD
	}
	if entry.GitBranch != "" {
		state.branch = entry.GitBranch
	}
	if entry.Entrypoint != "" && state.agentType == "" {
		state.agentType = entry.Entrypoint
	}
	if entry.Version != "" {
		state.version = entry.Version
	}
	if entry.Slug != "" {
		state.slug = entry.Slug
	}
}

// ensureHourBucket lazily allocates an HourlyActivity entry for hb.
func ensureHourBucket(activity map[string]*store.HourlyActivity, sessionID, projectKey, hb string) {
	if hb == "" || activity[hb] != nil {
		return
	}
	activity[hb] = &store.HourlyActivity{
		BrainID:    sessionID,
		Hour:       hb,
		ProjectKey: projectKey,
	}
}

// dispatchEntry routes a parsed JSONL line to the per-type handler.
func dispatchEntry(
	entry jsonlLine,
	hb string,
	activity map[string]*store.HourlyActivity,
	agentMsgs map[string]*store.AgentMessage,
	state *parseState,
	sessionID, projectKey string,
) {
	switch entry.Type {
	case "user":
		processUserEntry(entry, hb, activity, agentMsgs, state)
	case "assistant":
		processAssistantEntry(entry, hb, activity, agentMsgs, state, sessionID, projectKey)
	case "tool_use", "tool_result":
		if hb != "" {
			activity[hb].ToolUses++
		}
	}
}

// processUserEntry handles a single "user"-type JSONL line: increments the
// user message count, captures the first user message as the summary candidate,
// and walks content blocks for tool_result back-references that match an
// outstanding Agent tool_use.
func processUserEntry(
	entry jsonlLine,
	hb string,
	activity map[string]*store.HourlyActivity,
	agentMsgs map[string]*store.AgentMessage,
	state *parseState,
) {
	state.msgCount++
	if hb != "" {
		activity[hb].UserMsgs++
	}
	if entry.Message == nil {
		return
	}
	if state.firstMsg == "" {
		state.firstMsg = extractTextContent(entry.Message.Content)
	}
	// A "user" entry in the JSONL is also where tool_result blocks live
	// (they're the response side of a tool call). Scan for any tool_result
	// blocks that match an Agent tool_use we've seen.
	for _, b := range decodeContentBlocks(entry.Message.Content) {
		linkToolResult(b, agentMsgs)
	}
}

// linkToolResult attaches a tool_result's content to the agent invocation it
// responds to, if any. Tool_result blocks for non-Agent tools (Read, Bash,
// Edit, etc.) are silently ignored.
func linkToolResult(b contentBlock, agentMsgs map[string]*store.AgentMessage) {
	if b.Type != "tool_result" || b.ToolUseID == "" {
		return
	}
	msg, ok := agentMsgs[b.ToolUseID]
	if !ok {
		return
	}
	msg.Response = extractTextContent(b.Content)
}

// processAssistantEntry handles a single "assistant"-type JSONL line: bumps
// the assistant message count, records the model + token usage, and scans
// content blocks for Agent tool_use invocations that we want to capture as
// AgentMessage records.
func processAssistantEntry(
	entry jsonlLine,
	hb string,
	activity map[string]*store.HourlyActivity,
	agentMsgs map[string]*store.AgentMessage,
	state *parseState,
	sessionID, projectKey string,
) {
	state.msgCount++
	if hb != "" {
		activity[hb].AsstMsgs++
	}
	if entry.Message == nil {
		return
	}
	if entry.Message.Model != "" {
		state.model = entry.Message.Model
	}
	addUsage(state, hb, activity, entry.Message.Usage)
	for _, b := range decodeContentBlocks(entry.Message.Content) {
		captureAgentInvocation(b, agentMsgs, sessionID, projectKey, entry.Timestamp)
	}
}

// addUsage folds an assistant message's token usage into the running totals
// and the matching hour bucket (if any).
func addUsage(state *parseState, hb string, activity map[string]*store.HourlyActivity, usage *usagePayload) {
	if usage == nil {
		return
	}
	lineTokens := usage.InputTokens + usage.OutputTokens
	state.tokens += lineTokens
	if hb != "" {
		activity[hb].Tokens += lineTokens
	}
}

// captureAgentInvocation extracts an Agent tool_use invocation from a content
// block and stores it under its id for later matching against tool_result
// responses. Blocks that are not Agent invocations or that fail to decode are
// skipped silently.
func captureAgentInvocation(
	b contentBlock,
	agentMsgs map[string]*store.AgentMessage,
	sessionID, projectKey, timestamp string,
) {
	if b.Type != "tool_use" || b.Name != "Agent" || b.ID == "" {
		return
	}
	var in agentToolUseInput
	if err := json.Unmarshal(b.Input, &in); err != nil || in.SubagentType == "" {
		return
	}
	agentMsgs[b.ID] = &store.AgentMessage{
		ID:          b.ID,
		BrainID:     sessionID,
		AgentName:   in.SubagentType,
		Description: in.Description,
		Prompt:      in.Prompt,
		Timestamp:   timestamp,
		ProjectKey:  projectKey,
	}
}

// assembleBrain copies the accumulated parseState into the Brain and flattens
// the captured agent-message map into a slice for return.
func assembleBrain(
	b *store.Brain,
	state *parseState,
	agentMsgs map[string]*store.AgentMessage,
	newOffset int64,
) []store.AgentMessage {
	b.ProjectPath = state.cwd
	b.AgentType = state.agentType
	b.Model = state.model
	b.GitBranch = state.branch
	b.MessageCount = state.msgCount
	b.FirstMessageAt = state.firstTs
	b.LastMessageAt = state.lastTs
	b.Summary = truncate(state.firstMsg, 200)
	b.TokenUsage = state.tokens
	b.LastOffset = newOffset
	b.Slug = state.slug
	b.Version = state.version

	agentMessages := make([]store.AgentMessage, 0, len(agentMsgs))
	for _, m := range agentMsgs {
		agentMessages = append(agentMessages, *m)
	}
	return agentMessages
}

// decodeContentBlocks decodes a message.content field into an array of
// content blocks. Claude Code JSONL stores content as either:
//   - a string (legacy / simple text message)
//   - an array of typed blocks (modern — tool_use, tool_result, text, etc.)
//
// Returns an empty slice when the content is a plain string (no blocks to inspect).
func decodeContentBlocks(raw json.RawMessage) []contentBlock {
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	return blocks
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
