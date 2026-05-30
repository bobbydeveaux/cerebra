package brain

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/store"
)

func TestNewWatcher(t *testing.T) {
	t.Parallel()
	fs := newFakeStore()
	w := NewWatcher(fs, nil, "/some/path")
	if w == nil {
		t.Fatal("NewWatcher returned nil")
	}
	if w.watchPath != "/some/path" {
		t.Errorf("watchPath = %q, want /some/path", w.watchPath)
	}
	if w.pending == nil {
		t.Error("pending map not initialised")
	}
}

func TestWatcher_InitialScan(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// project A with two sessions; project B with one; one stray file ignored.
	projA := filepath.Join(root, "proj-a")
	projB := filepath.Join(root, "proj-b")
	if err := os.MkdirAll(projA, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(projB, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mustWrite := func(path, body string) {
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	mustWrite(filepath.Join(projA, "session-aaaaaaaa.jsonl"),
		`{"type":"user","timestamp":"2026-04-28T14:00:00Z","message":{"role":"user","content":"hello"}}`+"\n",
	)
	mustWrite(filepath.Join(projA, "session-bbbbbbbb.jsonl"),
		`{"type":"assistant","timestamp":"2026-04-28T14:30:00Z","message":{"role":"assistant","content":[{"type":"text","text":"world"}],"model":"x"}}`+"\n",
	)
	mustWrite(filepath.Join(projB, "session-cccccccc.jsonl"),
		`{"type":"user","timestamp":"2026-04-28T15:00:00Z","message":{"role":"user","content":"third"}}`+"\n",
	)
	// Ignored files: not .jsonl, plus a stray top-level file.
	mustWrite(filepath.Join(projA, "README.md"), "skipped\n")
	mustWrite(filepath.Join(root, "stray.jsonl"), `{"type":"user","message":{"content":"ignored"}}`+"\n")

	fs := newFakeStore()
	w := NewWatcher(fs, nil, root)
	count, err := w.initialScan(context.Background())
	if err != nil {
		t.Fatalf("initialScan: %v", err)
	}
	if count != 3 {
		t.Errorf("initialScan count = %d, want 3", count)
	}
	if len(fs.brains) != 3 {
		t.Errorf("brains stored = %d, want 3", len(fs.brains))
	}
}

func TestWatcher_InitialScan_RootMissing(t *testing.T) {
	t.Parallel()
	fs := newFakeStore()
	w := NewWatcher(fs, nil, filepath.Join(t.TempDir(), "does-not-exist"))
	_, err := w.initialScan(context.Background())
	if err == nil {
		t.Fatal("expected error when watch root missing")
	}
}

func TestWatcher_ProcessFile_FullRead(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "session-12345678.jsonl")
	content := strings.Join([]string{
		`{"type":"user","timestamp":"2026-04-28T14:00:00Z","cwd":"/repo","message":{"role":"user","content":"hi"}}`,
		`{"type":"assistant","timestamp":"2026-04-28T14:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"yo"}],"model":"opus","usage":{"input_tokens":1,"output_tokens":2}}}`,
		`{"type":"assistant","timestamp":"2026-04-28T14:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"tu_1","name":"Agent","input":{"subagent_type":"gopher","prompt":"go test"}}],"model":"opus"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := newFakeStore()
	w := NewWatcher(fs, nil, dir)

	if err := w.processFile(context.Background(), path, 0, false); err != nil {
		t.Fatalf("processFile: %v", err)
	}

	if len(fs.brains) != 1 {
		t.Fatalf("brains = %d, want 1", len(fs.brains))
	}
	if len(fs.activity) == 0 {
		t.Error("expected at least one activity bucket persisted")
	}
	if len(fs.agentMessages) != 1 {
		t.Errorf("agentMessages = %d, want 1", len(fs.agentMessages))
	}
	// full read with activity should have triggered activity-deletion before re-persist
	if len(fs.activityDeleted) != 1 {
		t.Errorf("activityDeleted = %d, want 1 (full read must clear stale buckets)", len(fs.activityDeleted))
	}
}

func TestWatcher_ProcessFile_EmptyFileSkipped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := newFakeStore()
	w := NewWatcher(fs, nil, dir)
	if err := w.processFile(context.Background(), path, 0, false); err != nil {
		t.Fatalf("processFile: %v", err)
	}
	if len(fs.brains) != 0 {
		t.Errorf("empty file should not create a brain; got %d", len(fs.brains))
	}
}

func TestWatcher_ProcessFile_Incremental(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "incr-session-id.jsonl")
	if err := os.WriteFile(path, []byte(
		`{"type":"user","timestamp":"2026-04-28T14:00:00Z","message":{"role":"user","content":"first"}}`+"\n",
	), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := newFakeStore()
	w := NewWatcher(fs, nil, dir)
	if err := w.processFile(context.Background(), path, 0, false); err != nil {
		t.Fatalf("first processFile: %v", err)
	}

	stored := fs.brains["incr-session-id"]
	if stored == nil {
		t.Fatal("expected stored brain")
	}
	firstCount := stored.MessageCount
	offset := stored.LastOffset

	// Append a new line then call processFile with the offset (simulating watcher delta).
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open append: %v", err)
	}
	if _, err := f.WriteString(
		`{"type":"assistant","timestamp":"2026-04-28T14:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"second"}],"model":"opus","usage":{"input_tokens":3,"output_tokens":4}}}` + "\n",
	); err != nil {
		t.Fatalf("append: %v", err)
	}
	f.Close()

	if err := w.processFile(context.Background(), path, offset, false); err != nil {
		t.Fatalf("incremental processFile: %v", err)
	}
	stored = fs.brains["incr-session-id"]
	if stored.MessageCount != firstCount+1 {
		t.Errorf("incremental: MessageCount = %d, want %d (merged)", stored.MessageCount, firstCount+1)
	}
	if stored.TokenUsage != 7 {
		t.Errorf("incremental: TokenUsage = %d, want 7", stored.TokenUsage)
	}
}

func TestWatcher_ProcessFile_ParseError(t *testing.T) {
	t.Parallel()
	fs := newFakeStore()
	w := NewWatcher(fs, nil, t.TempDir())
	err := w.processFile(context.Background(), filepath.Join(t.TempDir(), "absent.jsonl"), 0, false)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWatcher_ProcessFile_UpsertBrainError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "boom-session-id.jsonl")
	if err := os.WriteFile(path, []byte(
		`{"type":"user","timestamp":"2026-04-28T14:00:00Z","message":{"role":"user","content":"x"}}`+"\n",
	), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fs := newFakeStore()
	fs.upsertBrainErr = errors.New("db locked")

	w := NewWatcher(fs, nil, dir)
	err := w.processFile(context.Background(), path, 0, false)
	if err == nil {
		t.Fatal("expected upsert error")
	}
}

func TestWatcher_MarkStaleBrains(t *testing.T) {
	t.Parallel()
	fs := newFakeStore()

	stale := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339Nano)
	fresh := time.Now().UTC().Format(time.RFC3339Nano)
	fs.brains["stale"] = &store.Brain{BrainID: "stale", Status: StatusActive, LastMessageAt: stale}
	fs.brains["fresh"] = &store.Brain{BrainID: "fresh", Status: StatusActive, LastMessageAt: fresh}
	fs.brains["no-ts"] = &store.Brain{BrainID: "no-ts", Status: StatusActive, LastMessageAt: ""}
	fs.brains["bad-ts"] = &store.Brain{BrainID: "bad-ts", Status: StatusActive, LastMessageAt: "not-a-date"}

	w := NewWatcher(fs, nil, t.TempDir())
	w.markStaleBrains(context.Background())

	if fs.brains["stale"].Status != StatusCompleted {
		t.Errorf("stale brain status = %q, want %q", fs.brains["stale"].Status, StatusCompleted)
	}
	if fs.brains["fresh"].Status != StatusActive {
		t.Errorf("fresh brain status = %q, want %q", fs.brains["fresh"].Status, StatusActive)
	}
	if fs.brains["no-ts"].Status != StatusActive {
		t.Errorf("no-ts brain status = %q, want %q (skip on empty)", fs.brains["no-ts"].Status, StatusActive)
	}
	if fs.brains["bad-ts"].Status != StatusActive {
		t.Errorf("bad-ts brain status = %q, want %q (skip on parse fail)", fs.brains["bad-ts"].Status, StatusActive)
	}
}

func TestWatcher_MarkStaleBrains_AltTimestampFormat(t *testing.T) {
	t.Parallel()
	fs := newFakeStore()
	// "2006-01-02T15:04:05.000Z" alt format, 2 hours ago.
	staleTs := time.Now().Add(-2 * time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")
	fs.brains["alt"] = &store.Brain{BrainID: "alt", Status: StatusActive, LastMessageAt: staleTs}

	w := NewWatcher(fs, nil, t.TempDir())
	w.markStaleBrains(context.Background())

	if fs.brains["alt"].Status != StatusCompleted {
		t.Errorf("alt-format stale brain status = %q, want %q", fs.brains["alt"].Status, StatusCompleted)
	}
}

func TestWatcher_MarkStaleBrains_ListError(t *testing.T) {
	t.Parallel()
	fs := newFakeStore()
	fs.listBrainsErr = errors.New("db down")

	w := NewWatcher(fs, nil, t.TempDir())
	// Should not panic — error is logged and we return.
	w.markStaleBrains(context.Background())
}
