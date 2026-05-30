package brain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/store"
)

func TestHourBucket(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"rfc3339nano", "2026-04-28T14:32:11.123456789Z", "2026-04-28T14"},
		{"rfc3339", "2026-04-28T14:32:11Z", "2026-04-28T14"},
		{"alt-millis", "2026-04-28T14:32:11.000Z", "2026-04-28T14"},
		{"empty", "", ""},
		{"junk", "not-a-timestamp", ""},
		{"hour-boundary", "2026-04-28T23:59:59.999Z", "2026-04-28T23"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := hourBucket(c.in)
			if got != c.want {
				t.Errorf("hourBucket(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		in     string
		maxLen int
		want   string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"too-long", "hello world", 8, "hello..."},
		{"newlines-stripped", "hello\nworld\n", 20, "hello world"},
		{"leading-trailing-ws", "  trim me  ", 20, "trim me"},
		{"empty", "", 5, ""},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := truncate(c.in, c.maxLen)
			if got != c.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.maxLen, got, c.want)
			}
		})
	}
}

func TestExtractTextContent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"string", `"hello world"`, "hello world"},
		{"single-text-block", `[{"type":"text","text":"first text"}]`, "first text"},
		{
			name: "text-after-tool-use",
			raw:  `[{"type":"tool_use","id":"tu_1"},{"type":"text","text":"hello"}]`,
			want: "hello",
		},
		{"non-text-only", `[{"type":"tool_use","id":"x"}]`, ""},
		{"malformed", `not-json`, ""},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := extractTextContent(json.RawMessage(c.raw))
			if got != c.want {
				t.Errorf("extractTextContent(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

func TestDecodeContentBlocks(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		raw       string
		wantCount int
		check     func(t *testing.T, blocks []contentBlock)
	}{
		{
			name:      "empty",
			raw:       "",
			wantCount: 0,
		},
		{
			name:      "string-not-array",
			raw:       `"hello"`,
			wantCount: 0,
		},
		{
			name:      "single-text-block",
			raw:       `[{"type":"text","text":"hi"}]`,
			wantCount: 1,
			check: func(t *testing.T, blocks []contentBlock) {
				if blocks[0].Type != "text" || blocks[0].Text != "hi" {
					t.Errorf("unexpected block: %+v", blocks[0])
				}
			},
		},
		{
			name:      "tool-use-and-result",
			raw:       `[{"type":"tool_use","id":"tu_1","name":"Agent","input":{}},{"type":"tool_result","tool_use_id":"tu_1","content":"ok"}]`,
			wantCount: 2,
			check: func(t *testing.T, blocks []contentBlock) {
				if blocks[0].Name != "Agent" || blocks[0].ID != "tu_1" {
					t.Errorf("block[0] = %+v", blocks[0])
				}
				if blocks[1].ToolUseID != "tu_1" {
					t.Errorf("block[1] = %+v", blocks[1])
				}
			},
		},
		{
			name:      "malformed-array",
			raw:       `[not-json]`,
			wantCount: 0,
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			blocks := decodeContentBlocks(json.RawMessage(c.raw))
			if len(blocks) != c.wantCount {
				t.Fatalf("got %d blocks, want %d (raw=%q)", len(blocks), c.wantCount, c.raw)
			}
			if c.check != nil {
				c.check(t, blocks)
			}
		})
	}
}

func TestMergeIncremental(t *testing.T) {
	t.Parallel()

	existing := &store.Brain{
		BrainID:       "abc",
		MessageCount:  10,
		TokenUsage:    1000,
		LastOffset:    100,
		LastMessageAt: "old-ts",
		Status:        StatusCompleted,
	}
	delta := &store.Brain{
		MessageCount:  5,
		TokenUsage:    500,
		LastOffset:    250,
		LastMessageAt: "new-ts",
		Model:         "claude-opus-4-7",
		GitBranch:     "main",
		Version:       "1.0",
		Slug:          "my-session",
		Summary:       "delta summary",
		ProjectPath:   "/path",
		AgentType:     "claude-code",
	}
	MergeIncremental(existing, delta)

	if existing.MessageCount != 15 {
		t.Errorf("MessageCount = %d, want 15", existing.MessageCount)
	}
	if existing.TokenUsage != 1500 {
		t.Errorf("TokenUsage = %d, want 1500", existing.TokenUsage)
	}
	if existing.LastOffset != 250 {
		t.Errorf("LastOffset = %d, want 250", existing.LastOffset)
	}
	if existing.LastMessageAt != "new-ts" {
		t.Errorf("LastMessageAt = %q, want new-ts", existing.LastMessageAt)
	}
	if existing.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q, want claude-opus-4-7", existing.Model)
	}
	if existing.Status != StatusActive {
		t.Errorf("Status = %q, want %q (must flip to active on merge)", existing.Status, StatusActive)
	}
	if existing.Summary != "delta summary" {
		t.Errorf("Summary = %q, want 'delta summary' (existing was empty)", existing.Summary)
	}
	if existing.ProjectPath != "/path" {
		t.Errorf("ProjectPath = %q, want /path", existing.ProjectPath)
	}
}

func TestMergeIncremental_PreservesExisting(t *testing.T) {
	t.Parallel()
	existing := &store.Brain{
		BrainID:     "abc",
		Summary:     "keep me",
		ProjectPath: "/keep",
		AgentType:   "claude-code",
	}
	delta := &store.Brain{
		Summary:     "ignored",
		ProjectPath: "/ignored",
		AgentType:   "ignored",
	}
	MergeIncremental(existing, delta)

	if existing.Summary != "keep me" {
		t.Errorf("Summary = %q, want 'keep me' (existing must win)", existing.Summary)
	}
	if existing.ProjectPath != "/keep" {
		t.Errorf("ProjectPath = %q, want /keep", existing.ProjectPath)
	}
	if existing.AgentType != "claude-code" {
		t.Errorf("AgentType = %q, want claude-code", existing.AgentType)
	}
}

func TestParseSessionFile_HappyPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	lines := []string{
		`{"type":"user","timestamp":"2026-04-28T14:32:11.123Z","sessionId":"s1","cwd":"/repo","gitBranch":"main","entrypoint":"claude-code","version":"1.0","slug":"intro","message":{"role":"user","content":"hello"}}`,
		`{"type":"assistant","timestamp":"2026-04-28T14:32:12.123Z","message":{"role":"assistant","content":[{"type":"text","text":"hi there"}],"model":"claude-opus-4-7","usage":{"input_tokens":10,"output_tokens":20,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		`{"type":"user","timestamp":"2026-04-28T15:00:00.000Z","message":{"role":"user","content":"follow up"}}`,
		`{"type":"assistant","timestamp":"2026-04-28T15:00:01.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_1","name":"Agent","input":{"subagent_type":"gopher","description":"go test","prompt":"run tests"}}],"model":"claude-opus-4-7","usage":{"input_tokens":5,"output_tokens":5}}}`,
		`{"type":"user","timestamp":"2026-04-28T15:00:02.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu_1","content":"all green"}]}}`,
		`{}`, // skipped (no type)
		`malformed-line`, // skipped silently
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	b, activity, agentMsgs, offset, err := ParseSessionFile(path, 0)
	if err != nil {
		t.Fatalf("ParseSessionFile: %v", err)
	}
	if offset <= 0 {
		t.Errorf("expected positive offset, got %d", offset)
	}
	if b.BrainID != "session" {
		t.Errorf("BrainID = %q, want session (trim .jsonl)", b.BrainID)
	}
	if b.MessageCount != 5 { // 3 users + 2 assistants
		t.Errorf("MessageCount = %d, want 5", b.MessageCount)
	}
	if b.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q, want claude-opus-4-7", b.Model)
	}
	if b.AgentType != "claude-code" {
		t.Errorf("AgentType = %q, want claude-code", b.AgentType)
	}
	if b.GitBranch != "main" {
		t.Errorf("GitBranch = %q, want main", b.GitBranch)
	}
	if b.ProjectPath != "/repo" {
		t.Errorf("ProjectPath = %q, want /repo", b.ProjectPath)
	}
	if b.Status != StatusActive {
		t.Errorf("Status = %q, want %q", b.Status, StatusActive)
	}
	if b.Slug != "intro" {
		t.Errorf("Slug = %q, want intro", b.Slug)
	}
	if b.Version != "1.0" {
		t.Errorf("Version = %q, want 1.0", b.Version)
	}
	if b.TokenUsage != 40 { // (10+20) + (5+5)
		t.Errorf("TokenUsage = %d, want 40", b.TokenUsage)
	}
	if b.Summary != "hello" {
		t.Errorf("Summary = %q, want hello", b.Summary)
	}
	if b.FirstMessageAt == "" || b.LastMessageAt == "" {
		t.Errorf("timestamps not set: first=%q last=%q", b.FirstMessageAt, b.LastMessageAt)
	}

	// Activity buckets: two hours touched.
	if len(activity) != 2 {
		t.Errorf("activity buckets = %d, want 2", len(activity))
	}
	bucket14, ok := activity["2026-04-28T14"]
	if !ok {
		t.Fatal("missing 14:00 activity bucket")
	}
	if bucket14.UserMsgs != 1 || bucket14.AsstMsgs != 1 {
		t.Errorf("14:00 bucket: user=%d asst=%d, want 1/1", bucket14.UserMsgs, bucket14.AsstMsgs)
	}
	if bucket14.Tokens != 30 {
		t.Errorf("14:00 bucket: tokens=%d, want 30", bucket14.Tokens)
	}

	// Agent message recorded with response stitched from tool_result.
	if len(agentMsgs) != 1 {
		t.Fatalf("agentMsgs = %d, want 1", len(agentMsgs))
	}
	if agentMsgs[0].AgentName != "gopher" {
		t.Errorf("agent name = %q, want gopher", agentMsgs[0].AgentName)
	}
	if agentMsgs[0].Prompt != "run tests" {
		t.Errorf("agent prompt = %q, want 'run tests'", agentMsgs[0].Prompt)
	}
	if agentMsgs[0].Response != "all green" {
		t.Errorf("agent response = %q, want 'all green' (must stitch tool_result)", agentMsgs[0].Response)
	}
	if agentMsgs[0].BrainID != "session" {
		t.Errorf("agent brainID = %q, want session", agentMsgs[0].BrainID)
	}
}

func TestParseSessionFile_IncrementalOffset(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "incr.jsonl")

	line1 := `{"type":"user","timestamp":"2026-04-28T14:00:00Z","message":{"role":"user","content":"first"}}`
	line2 := `{"type":"assistant","timestamp":"2026-04-28T14:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"reply"}],"model":"x"}}`

	if err := os.WriteFile(path, []byte(line1+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	b1, _, _, offset, err := ParseSessionFile(path, 0)
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}
	if b1.MessageCount != 1 {
		t.Errorf("first parse: MessageCount = %d, want 1", b1.MessageCount)
	}

	// Append second line and re-parse from offset.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open append: %v", err)
	}
	if _, err := f.WriteString(line2 + "\n"); err != nil {
		t.Fatalf("append: %v", err)
	}
	f.Close()

	b2, _, _, newOffset, err := ParseSessionFile(path, offset)
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}
	if b2.MessageCount != 1 { // only the new assistant line, not 2
		t.Errorf("incremental: MessageCount = %d, want 1 (delta only)", b2.MessageCount)
	}
	if newOffset <= offset {
		t.Errorf("offset did not advance: %d -> %d", offset, newOffset)
	}
}

func TestParseSessionFile_FileMissing(t *testing.T) {
	t.Parallel()
	_, _, _, _, err := ParseSessionFile(filepath.Join(t.TempDir(), "absent.jsonl"), 0)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseSessionFile_SeekPastEnd(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "tiny.jsonl")
	if err := os.WriteFile(path, []byte(`{"type":"user"}`+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Seek well past EOF — Go allows this on regular files; the scanner
	// should just produce zero lines and return a brain with no messages.
	b, activity, agentMsgs, _, err := ParseSessionFile(path, 9999)
	if err != nil {
		t.Fatalf("ParseSessionFile: %v", err)
	}
	if b.MessageCount != 0 {
		t.Errorf("seek-past-eof: MessageCount = %d, want 0", b.MessageCount)
	}
	if len(activity) != 0 {
		t.Errorf("seek-past-eof: activity = %d, want 0", len(activity))
	}
	if len(agentMsgs) != 0 {
		t.Errorf("seek-past-eof: agentMsgs = %d, want 0", len(agentMsgs))
	}
}
