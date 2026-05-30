package brain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

func TestStripThinkingBlocks(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no-block", "plain text", "plain text"},
		{"single-block", "before <think>internal</think> after", "before  after"},
		{"two-blocks", "a <think>x</think> b <think>y</think> c", "a  b  c"},
		{"unclosed", "ok <think>oops", "ok"},
		{"adjacent", "<think>a</think><think>b</think>done", "done"},
		{"empty-block", "x <think></think> y", "x  y"},
		{"trims-result", "   <think>x</think>   ", ""},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := stripThinkingBlocks(c.in)
			if got != c.want {
				t.Errorf("stripThinkingBlocks(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestExtractMessageText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"string", `"hello"`, "hello"},
		{
			name: "joined-text-blocks",
			raw:  `[{"type":"text","text":"one"},{"type":"text","text":"two"}]`,
			want: "one\ntwo",
		},
		{
			name: "mixed-skip-non-text",
			raw:  `[{"type":"tool_use","id":"x"},{"type":"text","text":"only-text"}]`,
			want: "only-text",
		},
		{"empty-array", `[]`, ""},
		{"malformed", `not-json`, ""},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := extractMessageText(json.RawMessage(c.raw))
			if got != c.want {
				t.Errorf("extractMessageText(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

func TestExtractConversationText(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "conv.jsonl")

	lines := []string{
		`{"type":"user","message":{"role":"user","content":"first user message that is long enough"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"<think>plan</think>actual reply long enough"}]}}`,
		`{"type":"user","message":{"role":"user","content":"abc"}}`, // too short, skipped (<5 chars)
		`{"type":"summary","message":{"role":"system","content":"ignored"}}`,                              // wrong type
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1"}]}}`, // no text → skipped
		`bad-json`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := extractConversationText(path)
	if err != nil {
		t.Fatalf("extractConversationText: %v", err)
	}
	if !strings.Contains(got, "## User (message 1)") {
		t.Errorf("missing user header in output: %q", got)
	}
	if !strings.Contains(got, "first user message that is long enough") {
		t.Errorf("missing user body in output: %q", got)
	}
	if !strings.Contains(got, "## Assistant (message 2)") {
		t.Errorf("missing assistant header in output: %q", got)
	}
	if !strings.Contains(got, "actual reply long enough") {
		t.Errorf("missing assistant body in output: %q", got)
	}
	if strings.Contains(got, "plan") {
		t.Errorf("thinking block leaked into output: %q", got)
	}
	if strings.Contains(got, "ignored") {
		t.Errorf("system message leaked into output: %q", got)
	}
}

func TestExtractConversationText_FileMissing(t *testing.T) {
	t.Parallel()
	_, err := extractConversationText(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestIndexer_IndexBrain_HappyPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")

	lines := []string{
		`{"type":"user","message":{"role":"user","content":"a meaningful user question about something"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"a meaningful answer that is reasonably long"}]}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := newFakeStore()
	emb := &fakeEmbedder{vec: []float32{0.1, 0.2, 0.3, 0.4}}
	idx := NewIndexer(fs, emb, 1, 4, 256)

	brain := &store.Brain{
		BrainID:     "abc12345-deadbeef",
		ProjectPath: "/repo/path",
		ProjectKey:  "repo-path",
		SessionFile: path,
		Model:       "claude-opus-4-7",
		AgentType:   "claude-code",
		GitBranch:   "main",
	}

	if err := idx.IndexBrain(context.Background(), brain); err != nil {
		t.Fatalf("IndexBrain: %v", err)
	}

	if len(fs.upsertedDocs) != 1 {
		t.Fatalf("expected 1 upserted doc, got %d", len(fs.upsertedDocs))
	}
	for _, doc := range fs.upsertedDocs {
		if doc.FileType != scanner.FileTypeConversation {
			t.Errorf("FileType = %q, want %q", doc.FileType, scanner.FileTypeConversation)
		}
		if doc.SourceType != scanner.SourceTypeConversation {
			t.Errorf("SourceType = %q, want %q", doc.SourceType, scanner.SourceTypeConversation)
		}
		if doc.Repo != "path" { // filepath.Base("/repo/path")
			t.Errorf("Repo = %q, want path", doc.Repo)
		}
		if doc.Metadata["brain_id"] != brain.BrainID {
			t.Errorf("metadata brain_id = %q, want %q", doc.Metadata["brain_id"], brain.BrainID)
		}
		if doc.Metadata["model"] != "claude-opus-4-7" {
			t.Errorf("metadata model = %q, want claude-opus-4-7", doc.Metadata["model"])
		}
		if doc.ContentHash == "" {
			t.Errorf("missing content hash")
		}
		if !strings.Contains(doc.Content, "meaningful user question") {
			t.Errorf("content missing user body: %q", doc.Content)
		}
	}
}

func TestIndexer_IndexBrain_NoChangeShortCircuits(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "stable.jsonl")

	lines := []string{
		`{"type":"user","message":{"role":"user","content":"a meaningful user question about something"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"a meaningful answer"}]}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Pre-compute the hash IndexBrain will look for, and seed the fakeStore
	// so the no-change branch fires.
	content := func() string {
		s, err := extractConversationText(path)
		if err != nil {
			t.Fatalf("extract: %v", err)
		}
		return s
	}()
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

	brain := &store.Brain{
		BrainID:     "stable-brain",
		ProjectPath: "/repo",
		SessionFile: path,
	}
	docID := fmt.Sprintf("%x", sha256.Sum256([]byte("brain:"+brain.BrainID)))

	fs := newFakeStore()
	fs.contentHashes[docID] = hash // pre-seed

	idx := NewIndexer(fs, &fakeEmbedder{vec: []float32{0.1}}, 1, 4, 256)
	if err := idx.IndexBrain(context.Background(), brain); err != nil {
		t.Fatalf("IndexBrain: %v", err)
	}

	if len(fs.upsertedDocs) != 0 {
		t.Errorf("no-change should not upsert; got %d docs", len(fs.upsertedDocs))
	}
}

func TestIndexer_IndexBrain_TooShortSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "tiny.jsonl")
	// All user messages under 5 chars → filtered → conversation text < 20 chars.
	if err := os.WriteFile(path, []byte(`{"type":"user","message":{"role":"user","content":"hi"}}`+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := newFakeStore()
	idx := NewIndexer(fs, &fakeEmbedder{vec: []float32{0.1}}, 1, 4, 256)
	brain := &store.Brain{BrainID: "tiny-brain-id", ProjectPath: "/r", SessionFile: path}

	if err := idx.IndexBrain(context.Background(), brain); err != nil {
		t.Fatalf("IndexBrain: %v", err)
	}
	if len(fs.upsertedDocs) != 0 {
		t.Errorf("expected zero upserts for too-short content; got %d", len(fs.upsertedDocs))
	}
}

func TestIndexer_IndexBrain_FileMissing(t *testing.T) {
	t.Parallel()
	fs := newFakeStore()
	idx := NewIndexer(fs, &fakeEmbedder{vec: []float32{0.1}}, 1, 4, 256)

	err := idx.IndexBrain(context.Background(), &store.Brain{
		BrainID:     "ghost",
		SessionFile: filepath.Join(t.TempDir(), "absent.jsonl"),
	})
	if err == nil {
		t.Fatal("expected error for missing session file")
	}
	if !strings.Contains(err.Error(), "extracting conversation") {
		t.Errorf("error should wrap with 'extracting conversation', got: %v", err)
	}
}

func TestIndexer_IndexBrain_EmbedError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"a meaningful user question about something"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"a meaningful answer that is reasonably long"}]}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := newFakeStore()
	emb := &fakeEmbedder{embedErr: errors.New("ollama unreachable")}
	idx := NewIndexer(fs, emb, 1, 4, 256)

	err := idx.IndexBrain(context.Background(), &store.Brain{
		BrainID:     "boom-brain-id",
		ProjectPath: "/repo",
		SessionFile: path,
	})
	if err == nil {
		t.Fatal("expected embed error")
	}
	if !strings.Contains(err.Error(), "embedding conversation") {
		t.Errorf("error should wrap with 'embedding conversation', got: %v", err)
	}
}

func TestIndexer_IndexBrain_StoreUpsertError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"a meaningful user question about something"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"a meaningful answer that is reasonably long"}]}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := newFakeStore()
	fs.upsertDocumentErr = errors.New("db locked")
	idx := NewIndexer(fs, &fakeEmbedder{vec: []float32{0.1}}, 1, 4, 256)

	err := idx.IndexBrain(context.Background(), &store.Brain{
		BrainID:     "bad-brain-id",
		ProjectPath: "/repo",
		SessionFile: path,
	})
	if err == nil {
		t.Fatal("expected store error")
	}
	if !strings.Contains(err.Error(), "storing conversation") {
		t.Errorf("error should wrap with 'storing conversation', got: %v", err)
	}
}

func TestIndexer_IndexBrain_ProjectNameFallback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"a meaningful user question about something"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"a meaningful answer that is reasonably long"}]}}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := newFakeStore()
	idx := NewIndexer(fs, &fakeEmbedder{vec: []float32{0.1}}, 1, 4, 256)

	// Empty ProjectPath → fall back to ProjectKey.
	brain := &store.Brain{
		BrainID:     "12345678-abcd",
		ProjectPath: "", // intentionally empty
		ProjectKey:  "fallback-key",
		SessionFile: path,
	}
	if err := idx.IndexBrain(context.Background(), brain); err != nil {
		t.Fatalf("IndexBrain: %v", err)
	}

	if len(fs.upsertedDocs) != 1 {
		t.Fatalf("expected 1 upserted doc, got %d", len(fs.upsertedDocs))
	}
	for _, doc := range fs.upsertedDocs {
		if doc.Repo != "fallback-key" {
			t.Errorf("Repo = %q, want fallback-key (ProjectKey fallback)", doc.Repo)
		}
		if !strings.HasPrefix(doc.RelPath, "brains/fallback-key/") {
			t.Errorf("RelPath = %q, want prefix brains/fallback-key/", doc.RelPath)
		}
	}
}
