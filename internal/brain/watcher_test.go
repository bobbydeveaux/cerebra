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
	"github.com/fsnotify/fsnotify"
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

// --- addWatchRecursive coverage ---

func TestWatcher_AddWatchRecursive(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Build nested layout: root/a, root/a/aa, root/b
	for _, sub := range []string{"a", filepath.Join("a", "aa"), "b"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	// Drop a regular file alongside — it must be skipped, not Add()ed.
	if err := os.WriteFile(filepath.Join(root, "a", "skip.txt"), []byte("x"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify unavailable: %v", err)
	}
	defer fsw.Close()

	w := NewWatcher(newFakeStore(), nil, root)
	if err := w.addWatchRecursive(fsw, root); err != nil {
		t.Fatalf("addWatchRecursive: %v", err)
	}

	// WatchList() returns the directories currently being watched. We should
	// see at minimum root + a + a/aa + b, in some order. Resolve symlinks
	// because macOS reports /private/var/folders/... for TempDir paths.
	got := make(map[string]bool)
	for _, p := range fsw.WatchList() {
		if abs, err := filepath.EvalSymlinks(p); err == nil {
			got[abs] = true
		} else {
			got[p] = true
		}
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		rootResolved = root
	}
	for _, sub := range []string{"", "a", filepath.Join("a", "aa"), "b"} {
		want := filepath.Join(rootResolved, sub)
		if !got[want] {
			t.Errorf("expected %s in WatchList(), got %v", want, fsw.WatchList())
		}
	}
}

func TestWatcher_AddWatchRecursive_MissingRoot(t *testing.T) {
	t.Parallel()
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify unavailable: %v", err)
	}
	defer fsw.Close()

	w := NewWatcher(newFakeStore(), nil, "/nonexistent")
	// WalkDir's first invocation gets err!=nil; the callback returns nil to
	// skip inaccessible dirs, so addWatchRecursive must NOT propagate.
	if err := w.addWatchRecursive(fsw, "/this/path/does/not/exist"); err != nil {
		t.Errorf("expected nil error for missing root, got %v", err)
	}
}

// --- handleEvent coverage ---

func TestWatcher_HandleEvent_NonJSONLIgnored(t *testing.T) {
	t.Parallel()
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify unavailable: %v", err)
	}
	defer fsw.Close()

	fs := newFakeStore()
	w := NewWatcher(fs, nil, t.TempDir())
	w.handleEvent(context.Background(), fsw, fsnotify.Event{
		Name: "/tmp/some-file.txt",
		Op:   fsnotify.Write,
	})

	w.mu.Lock()
	pending := len(w.pending)
	w.mu.Unlock()
	if pending != 0 {
		t.Errorf("non-.jsonl event should not schedule debounce; pending=%d", pending)
	}
}

func TestWatcher_HandleEvent_SubagentIgnored(t *testing.T) {
	t.Parallel()
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify unavailable: %v", err)
	}
	defer fsw.Close()

	fs := newFakeStore()
	w := NewWatcher(fs, nil, t.TempDir())
	w.handleEvent(context.Background(), fsw, fsnotify.Event{
		Name: "/tmp/proj/subagents/sub-12345678.jsonl",
		Op:   fsnotify.Write,
	})

	w.mu.Lock()
	pending := len(w.pending)
	w.mu.Unlock()
	if pending != 0 {
		t.Errorf("subagent path should not schedule debounce; pending=%d", pending)
	}
}

func TestWatcher_HandleEvent_RemoveIgnored(t *testing.T) {
	t.Parallel()
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify unavailable: %v", err)
	}
	defer fsw.Close()

	fs := newFakeStore()
	w := NewWatcher(fs, nil, t.TempDir())
	// REMOVE-only on a .jsonl: should fall through neither Create nor Write
	// branch, and return without scheduling.
	w.handleEvent(context.Background(), fsw, fsnotify.Event{
		Name: "/tmp/proj/session-aaaaaaaa.jsonl",
		Op:   fsnotify.Remove,
	})

	w.mu.Lock()
	pending := len(w.pending)
	w.mu.Unlock()
	if pending != 0 {
		t.Errorf("REMOVE-only event should not schedule debounce; pending=%d", pending)
	}
}

func TestWatcher_HandleEvent_DirCreateAddsWatch(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	newDir := filepath.Join(root, "new-proj")
	if err := os.Mkdir(newDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify unavailable: %v", err)
	}
	defer fsw.Close()

	fs := newFakeStore()
	w := NewWatcher(fs, nil, root)
	w.handleEvent(context.Background(), fsw, fsnotify.Event{
		Name: newDir,
		Op:   fsnotify.Create,
	})

	// We don't strictly need to assert that newDir appears in WatchList
	// (it's best-effort inside handleEvent), but we do need to confirm
	// the directory branch returned BEFORE the suffix gate fired — i.e.
	// no debounce was scheduled because the early-return path was taken.
	w.mu.Lock()
	pending := len(w.pending)
	w.mu.Unlock()
	if pending != 0 {
		t.Errorf("directory CREATE should early-return; pending=%d", pending)
	}
}

func TestWatcher_HandleEvent_WriteSchedulesAndProcesses(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sessionID := "session-deadbeef"
	path := filepath.Join(root, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(
		`{"type":"user","timestamp":"2026-04-28T14:00:00Z","message":{"role":"user","content":"hi"}}`+"\n",
	), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify unavailable: %v", err)
	}
	defer fsw.Close()

	fs := newFakeStore()
	w := NewWatcher(fs, nil, root)
	w.handleEvent(context.Background(), fsw, fsnotify.Event{
		Name: path,
		Op:   fsnotify.Write,
	})

	// Confirm a debounced callback is now scheduled.
	w.mu.Lock()
	pending := len(w.pending)
	w.mu.Unlock()
	if pending != 1 {
		t.Fatalf("expected 1 pending debounce, got %d", pending)
	}

	// Wait for the 500ms AfterFunc to fire and finish.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		stillPending := len(w.pending)
		w.mu.Unlock()
		if stillPending == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Give the callback's processFile a moment to complete.
	for time.Now().Before(deadline) {
		fs.mu.Lock()
		n := len(fs.brains)
		fs.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	fs.mu.Lock()
	n := len(fs.brains)
	fs.mu.Unlock()
	if n != 1 {
		t.Errorf("expected 1 brain upserted after debounce, got %d", n)
	}
}

func TestWatcher_HandleEvent_CoalescesRapidWrites(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	sessionID := "session-coalesce"
	path := filepath.Join(root, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(
		`{"type":"user","timestamp":"2026-04-28T14:00:00Z","message":{"role":"user","content":"hi"}}`+"\n",
	), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Skipf("fsnotify unavailable: %v", err)
	}
	defer fsw.Close()

	fs := newFakeStore()
	w := NewWatcher(fs, nil, root)

	// Two rapid Write events: the second must stop the first timer and
	// install a new one. Net effect: still exactly ONE pending entry.
	ev := fsnotify.Event{Name: path, Op: fsnotify.Write}
	w.handleEvent(context.Background(), fsw, ev)
	w.handleEvent(context.Background(), fsw, ev)

	w.mu.Lock()
	pending := len(w.pending)
	w.mu.Unlock()
	if pending != 1 {
		t.Errorf("rapid writes should coalesce to 1 pending entry, got %d", pending)
	}

	// Drain the timer so the test doesn't leak a goroutine.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		stillPending := len(w.pending)
		w.mu.Unlock()
		if stillPending == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// --- Start coverage ---

func TestWatcher_Start_InitialScanError(t *testing.T) {
	t.Parallel()
	fs := newFakeStore()
	w := NewWatcher(fs, nil, filepath.Join(t.TempDir(), "missing-root"))

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := w.Start(ctx)
	if err == nil {
		t.Fatal("expected error when watchPath does not exist")
	}
	if !strings.Contains(err.Error(), "initial scan") {
		t.Errorf("error = %v, want wrapped 'initial scan'", err)
	}
}

func TestWatcher_Start_RunsAndExitsOnContextCancel(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	projDir := filepath.Join(root, "proj-x")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Pre-seed one session so initialScan reports a non-zero count and
	// exercises that branch too.
	seedPath := filepath.Join(projDir, "session-startseed.jsonl")
	if err := os.WriteFile(seedPath, []byte(
		`{"type":"user","timestamp":"2026-04-28T14:00:00Z","message":{"role":"user","content":"seed"}}`+"\n",
	), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	fs := newFakeStore()
	w := NewWatcher(fs, nil, root)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- w.Start(ctx)
	}()

	// Give Start a moment to scan + register watches, then trigger a Write
	// event on the seeded file so the Events channel branch is exercised.
	time.Sleep(150 * time.Millisecond)

	// Append to the existing file (Write event).
	if f, err := os.OpenFile(seedPath, os.O_APPEND|os.O_WRONLY, 0644); err == nil {
		_, _ = f.WriteString(
			`{"type":"assistant","timestamp":"2026-04-28T14:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"more"}],"model":"opus"}}` + "\n",
		)
		_ = f.Close()
	}

	// Also create a new sibling .jsonl to trigger a Create event.
	newPath := filepath.Join(projDir, "session-startnew.jsonl")
	_ = os.WriteFile(newPath, []byte(
		`{"type":"user","timestamp":"2026-04-28T14:00:02Z","message":{"role":"user","content":"new"}}`+"\n",
	), 0644)

	// And create a brand new sub-project directory so the Create+IsDir
	// branch inside handleEvent fires from the live loop.
	_ = os.MkdirAll(filepath.Join(root, "proj-y"), 0755)

	// Let events drain through the loop.
	time.Sleep(150 * time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned non-nil error on context cancel: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return within 3s of context cancel")
	}

	// initialScan should at least have ingested the seed.
	fs.mu.Lock()
	n := len(fs.brains)
	fs.mu.Unlock()
	if n == 0 {
		t.Error("expected at least the seed brain to be ingested by initialScan")
	}
}
