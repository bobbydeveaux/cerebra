package store

// brains_test.go covers the brains, activity, and agent_messages tables.
// Tests use the testDB helper from store_test.go and require the
// sqlite_fts5 build tag for FTS-backed agent_messages search.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
)

// --- brains.go ---------------------------------------------------------------

func TestUpsertAndGetBrain(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	now := time.Now().UTC().Format(time.RFC3339)
	b := Brain{
		BrainID:        "brain-1",
		ProjectPath:    "/Users/x/code/foo",
		ProjectKey:     "foo",
		SessionFile:    "/tmp/foo.jsonl",
		AgentType:      "cli",
		Model:          "claude-opus-4",
		GitBranch:      "main",
		Status:         "active",
		MessageCount:   12,
		FirstMessageAt: now,
		LastMessageAt:  now,
		Summary:        "did some work",
		TokenUsage:     1024,
		LastOffset:     2048,
		Slug:           "foo-slug",
		Version:        "1",
	}

	if err := s.UpsertBrain(ctx, b); err != nil {
		t.Fatalf("UpsertBrain: %v", err)
	}

	got, err := s.GetBrain(ctx, "brain-1")
	if err != nil {
		t.Fatalf("GetBrain: %v", err)
	}
	if got == nil {
		t.Fatal("GetBrain returned nil for known brain")
	}
	if got.BrainID != b.BrainID || got.ProjectKey != b.ProjectKey || got.MessageCount != 12 {
		t.Errorf("brain round-trip mismatch: %+v", got)
	}

	// Upsert again with new counts — should overwrite, not duplicate.
	b.MessageCount = 25
	b.TokenUsage = 3000
	b.Status = "completed"
	if err := s.UpsertBrain(ctx, b); err != nil {
		t.Fatalf("UpsertBrain (update): %v", err)
	}

	got2, err := s.GetBrain(ctx, "brain-1")
	if err != nil {
		t.Fatalf("GetBrain after update: %v", err)
	}
	if got2.MessageCount != 25 || got2.TokenUsage != 3000 || got2.Status != "completed" {
		t.Errorf("update did not persist: %+v", got2)
	}
}

func TestGetBrain_NotFound(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	got, err := s.GetBrain(ctx, "missing")
	if err != nil {
		t.Fatalf("GetBrain unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing brain, got %+v", got)
	}
}

func TestListBrains_FiltersAndLimit(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	seeds := []Brain{
		{BrainID: "b-foo-1", ProjectKey: "foo", ProjectPath: "/p/foo", SessionFile: "x", Status: "active", MessageCount: 1, LastMessageAt: "2026-05-30T10:00:00Z"},
		{BrainID: "b-foo-2", ProjectKey: "foo", ProjectPath: "/p/foo", SessionFile: "y", Status: "completed", MessageCount: 2, LastMessageAt: "2026-05-30T11:00:00Z"},
		{BrainID: "b-bar-1", ProjectKey: "bar", ProjectPath: "/p/bar", SessionFile: "z", Status: "active", MessageCount: 3, LastMessageAt: "2026-05-30T12:00:00Z"},
	}
	for _, b := range seeds {
		if err := s.UpsertBrain(ctx, b); err != nil {
			t.Fatalf("seed UpsertBrain %s: %v", b.BrainID, err)
		}
	}

	all, err := s.ListBrains(ctx, "", "", 0)
	if err != nil {
		t.Fatalf("ListBrains all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 brains, got %d", len(all))
	}

	fooOnly, err := s.ListBrains(ctx, "foo", "", 10)
	if err != nil {
		t.Fatalf("ListBrains foo: %v", err)
	}
	if len(fooOnly) != 2 {
		t.Errorf("expected 2 foo brains, got %d", len(fooOnly))
	}

	activeOnly, err := s.ListBrains(ctx, "", "active", 10)
	if err != nil {
		t.Fatalf("ListBrains active: %v", err)
	}
	if len(activeOnly) != 2 {
		t.Errorf("expected 2 active brains, got %d", len(activeOnly))
	}

	limit1, err := s.ListBrains(ctx, "", "", 1)
	if err != nil {
		t.Fatalf("ListBrains limit 1: %v", err)
	}
	if len(limit1) != 1 {
		t.Errorf("expected 1 brain with limit=1, got %d", len(limit1))
	}
	// Order is by last_message_at DESC — bar (12:00) should win.
	if limit1[0].BrainID != "b-bar-1" {
		t.Errorf("expected newest brain b-bar-1 first, got %s", limit1[0].BrainID)
	}
}

func TestGetBrainStats(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	empty, err := s.GetBrainStats(ctx)
	if err != nil {
		t.Fatalf("GetBrainStats empty: %v", err)
	}
	if empty.TotalBrains != 0 || empty.TotalMessages != 0 || empty.TotalTokens != 0 {
		t.Errorf("expected zero stats, got %+v", empty)
	}

	seeds := []Brain{
		{BrainID: "s-1", ProjectKey: "p1", ProjectPath: "/p1", SessionFile: "f1", Status: "active", MessageCount: 10, TokenUsage: 100},
		{BrainID: "s-2", ProjectKey: "p1", ProjectPath: "/p1", SessionFile: "f2", Status: "active", MessageCount: 20, TokenUsage: 200},
		{BrainID: "s-3", ProjectKey: "p2", ProjectPath: "/p2", SessionFile: "f3", Status: "completed", MessageCount: 5, TokenUsage: 50},
	}
	for _, b := range seeds {
		if err := s.UpsertBrain(ctx, b); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	stats, err := s.GetBrainStats(ctx)
	if err != nil {
		t.Fatalf("GetBrainStats: %v", err)
	}
	if stats.TotalBrains != 3 {
		t.Errorf("TotalBrains: want 3, got %d", stats.TotalBrains)
	}
	if stats.ActiveBrains != 2 {
		t.Errorf("ActiveBrains: want 2, got %d", stats.ActiveBrains)
	}
	if stats.TotalMessages != 35 {
		t.Errorf("TotalMessages: want 35, got %d", stats.TotalMessages)
	}
	if stats.TotalTokens != 350 {
		t.Errorf("TotalTokens: want 350, got %d", stats.TotalTokens)
	}
	if stats.Projects != 2 {
		t.Errorf("Projects: want 2, got %d", stats.Projects)
	}
}

// --- activity.go -------------------------------------------------------------

func TestActivity_UpsertAccumulatesAndDeletes(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	// Brain row is the FK target for brain_activity.
	if err := s.UpsertBrain(ctx, Brain{BrainID: "act-brain", ProjectKey: "actproj", ProjectPath: "/a", SessionFile: "f"}); err != nil {
		t.Fatalf("seed brain: %v", err)
	}

	first := HourlyActivity{
		BrainID: "act-brain", Hour: "2026-05-30T10", ProjectKey: "actproj",
		UserMsgs: 1, AsstMsgs: 2, ToolUses: 3, Tokens: 100,
	}
	if err := s.UpsertActivity(ctx, first); err != nil {
		t.Fatalf("UpsertActivity 1: %v", err)
	}

	// Second upsert on the same (brain_id, hour) should accumulate per the SQL.
	second := HourlyActivity{
		BrainID: "act-brain", Hour: "2026-05-30T10", ProjectKey: "actproj",
		UserMsgs: 4, AsstMsgs: 5, ToolUses: 6, Tokens: 200,
	}
	if err := s.UpsertActivity(ctx, second); err != nil {
		t.Fatalf("UpsertActivity 2: %v", err)
	}

	// A second hour for the same brain — keeps the first row, adds new row.
	third := HourlyActivity{
		BrainID: "act-brain", Hour: "2026-05-30T11", ProjectKey: "actproj",
		UserMsgs: 1, Tokens: 50,
	}
	if err := s.UpsertActivity(ctx, third); err != nil {
		t.Fatalf("UpsertActivity 3: %v", err)
	}

	rows, err := s.ListActivity(ctx, "actproj", "")
	if err != nil {
		t.Fatalf("ListActivity all: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 activity rows, got %d", len(rows))
	}
	// First row (hour 10) should be the accumulated total.
	if rows[0].UserMsgs != 5 || rows[0].AsstMsgs != 7 || rows[0].ToolUses != 9 || rows[0].Tokens != 300 {
		t.Errorf("accumulation wrong on hour 10: %+v", rows[0])
	}

	// Filter by date prefix.
	may30, err := s.ListActivity(ctx, "", "2026-05-30")
	if err != nil {
		t.Fatalf("ListActivity date: %v", err)
	}
	if len(may30) != 2 {
		t.Errorf("expected 2 rows for date 2026-05-30, got %d", len(may30))
	}

	// Filter to a date with no rows.
	may31, err := s.ListActivity(ctx, "", "2026-05-31")
	if err != nil {
		t.Fatalf("ListActivity empty date: %v", err)
	}
	if len(may31) != 0 {
		t.Errorf("expected 0 rows for 2026-05-31, got %d", len(may31))
	}

	// Delete activity by brain — both rows go.
	if err := s.DeleteBrainActivity(ctx, "act-brain"); err != nil {
		t.Fatalf("DeleteBrainActivity: %v", err)
	}
	after, err := s.ListActivity(ctx, "", "")
	if err != nil {
		t.Fatalf("ListActivity after delete: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("expected 0 rows after delete, got %d", len(after))
	}

	// Deleting an unknown brain is a no-op, not an error.
	if err := s.DeleteBrainActivity(ctx, "no-such-brain"); err != nil {
		t.Errorf("DeleteBrainActivity unknown: %v", err)
	}
}

// --- store.go uncovered branches --------------------------------------------

func TestScanState_RoundTripAndMissing(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	// Missing repo returns (nil, nil).
	state, err := s.GetScanState(ctx, "no-such-repo")
	if err != nil {
		t.Fatalf("GetScanState missing: %v", err)
	}
	if state != nil {
		t.Errorf("expected nil for missing scan state, got %+v", state)
	}

	// Seed a repo row first so the FK is valid, then upsert scan state.
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO repos (id, root_path) VALUES (?, ?)`,
		"repo-A", "/tmp/repo-A"); err != nil {
		t.Fatalf("seed repo: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	wanted := ScanState{
		Repo: "repo-A", LastCommitSHA: "deadbeef",
		LastScanTime: now, FileCount: 42, ChunkCount: 137,
	}
	if err := s.UpdateScanState(ctx, wanted); err != nil {
		t.Fatalf("UpdateScanState: %v", err)
	}

	got, err := s.GetScanState(ctx, "repo-A")
	if err != nil {
		t.Fatalf("GetScanState: %v", err)
	}
	if got == nil {
		t.Fatal("expected scan state, got nil")
	}
	if got.LastCommitSHA != "deadbeef" || got.FileCount != 42 || got.ChunkCount != 137 {
		t.Errorf("scan state mismatch: %+v", got)
	}

	// Update path — same repo_id, new sha + counts.
	updated := ScanState{
		Repo: "repo-A", LastCommitSHA: "cafebabe",
		LastScanTime: now.Add(time.Hour), FileCount: 50, ChunkCount: 200,
	}
	if err := s.UpdateScanState(ctx, updated); err != nil {
		t.Fatalf("UpdateScanState (update): %v", err)
	}
	got2, err := s.GetScanState(ctx, "repo-A")
	if err != nil {
		t.Fatalf("GetScanState (post-update): %v", err)
	}
	if got2.LastCommitSHA != "cafebabe" || got2.FileCount != 50 {
		t.Errorf("update did not persist: %+v", got2)
	}
}

func TestListFiles_ByCategoryAndAll(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	// Three docs across two categories.
	docs := []scanner.Document{
		{ID: "lf-1", Path: "/p/a.go", RelPath: "a.go", Category: scanner.CategoryAPI, Language: "go", FileType: scanner.FileTypeCode, Content: "x", ContentHash: "1", Metadata: map[string]string{}},
		{ID: "lf-2", Path: "/p/b.go", RelPath: "b.go", Category: scanner.CategoryAPI, Language: "go", FileType: scanner.FileTypeCode, Content: "x", ContentHash: "2", Metadata: map[string]string{}},
		{ID: "lf-3", Path: "/p/README.md", RelPath: "README.md", Category: scanner.CategoryDocs, Language: "", FileType: scanner.FileTypeMarkdown, Content: "x", ContentHash: "3", Metadata: map[string]string{}},
	}
	for _, d := range docs {
		if err := s.UpsertDocument(ctx, d, nil); err != nil {
			t.Fatalf("seed %s: %v", d.ID, err)
		}
	}

	apiFiles, err := s.ListFilesByCategory(ctx, string(scanner.CategoryAPI), 0)
	if err != nil {
		t.Fatalf("ListFilesByCategory: %v", err)
	}
	if len(apiFiles) != 2 {
		t.Errorf("expected 2 api files, got %d", len(apiFiles))
	}

	// Explicit limit.
	one, err := s.ListFilesByCategory(ctx, string(scanner.CategoryAPI), 1)
	if err != nil {
		t.Fatalf("ListFilesByCategory limit 1: %v", err)
	}
	if len(one) != 1 {
		t.Errorf("expected 1 with limit=1, got %d", len(one))
	}

	// Unknown category returns empty slice.
	none, err := s.ListFilesByCategory(ctx, "no-such-category", 10)
	if err != nil {
		t.Fatalf("ListFilesByCategory unknown: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 for unknown category, got %d", len(none))
	}

	all, err := s.ListAllFiles(ctx)
	if err != nil {
		t.Fatalf("ListAllFiles: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 total files, got %d", len(all))
	}
}

func TestUpsertDocument_ReplacesExistingChunks(t *testing.T) {
	s := testDB(t)
	if !s.ftsAvailable {
		t.Skip("FTS5 not built in; run: make test (uses -tags sqlite_fts5)")
	}
	ctx := context.Background()

	doc := scanner.Document{
		ID: "replace-doc", Path: "/p/r.go", RelPath: "r.go",
		Category: scanner.CategoryAPI, Language: "go", FileType: scanner.FileTypeCode,
		Content: "v1", ContentHash: "h1", Metadata: map[string]string{},
		Repo: "repo-R", RepoRoot: "/p",
	}
	v1Chunks := []chunker.Chunk{
		{ID: "rc-1", DocumentID: "replace-doc", Content: "old chunk 1", StartLine: 1, EndLine: 1},
		{ID: "rc-2", DocumentID: "replace-doc", Content: "old chunk 2", StartLine: 2, EndLine: 2},
	}
	if err := s.UpsertDocument(ctx, doc, v1Chunks); err != nil {
		t.Fatalf("UpsertDocument v1: %v", err)
	}

	// Replace with a different set of chunks (different IDs + count).
	doc.Content = "v2"
	doc.ContentHash = "h2"
	v2Chunks := []chunker.Chunk{
		{ID: "rc-3", DocumentID: "replace-doc", Content: "new chunk", StartLine: 1, EndLine: 1},
	}
	if err := s.UpsertDocument(ctx, doc, v2Chunks); err != nil {
		t.Fatalf("UpsertDocument v2: %v", err)
	}

	_, gotChunks, err := s.GetDocument(ctx, "r.go")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if len(gotChunks) != 1 {
		t.Fatalf("expected 1 chunk after replace, got %d", len(gotChunks))
	}
	if gotChunks[0].ID != "rc-3" {
		t.Errorf("expected new chunk rc-3, got %s", gotChunks[0].ID)
	}

	// Old chunk IDs should be gone from chunks_fts as well.
	var ftsCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM chunks_fts WHERE chunk_id IN ('rc-1','rc-2')`).Scan(&ftsCount); err != nil {
		t.Fatalf("count fts: %v", err)
	}
	if ftsCount != 0 {
		t.Errorf("expected 0 old fts rows, got %d", ftsCount)
	}
}

func TestSearchFTS_EscapesQuotes(t *testing.T) {
	s := testDB(t)
	if !s.ftsAvailable {
		t.Skip("FTS5 not built in; run: make test (uses -tags sqlite_fts5)")
	}
	ctx := context.Background()

	doc := scanner.Document{
		ID: "esc-1", Path: "/p/e.go", RelPath: "e.go",
		Category: scanner.CategoryUnknown, FileType: scanner.FileTypeCode,
		Content: `fmt.Println("quoted")`, ContentHash: "esc", Metadata: map[string]string{},
	}
	chunks := []chunker.Chunk{
		{ID: "esc-c1", DocumentID: "esc-1", Content: `fmt.Println("quoted")`, StartLine: 1, EndLine: 1},
	}
	if err := s.UpsertDocument(ctx, doc, chunks); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Pass a query containing a double quote — must not error.
	results, err := s.SearchFTS(ctx, `Println`, 5)
	if err != nil {
		t.Fatalf("SearchFTS: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least one hit for Println")
	}

	// Empty results don't error.
	empty, err := s.SearchFTS(ctx, "definitely-no-such-token-12345", 5)
	if err != nil {
		t.Fatalf("SearchFTS empty: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 results for unmatched term, got %d", len(empty))
	}

	// Default limit fires when limit <= 0.
	if _, err := s.SearchFTS(ctx, "Println", 0); err != nil {
		t.Errorf("SearchFTS default-limit path: %v", err)
	}
}

func TestSearch_DefaultLimitAndUnavailable(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if !s.vecAvailable {
		t.Skip("sqlite-vec extension not available")
	}

	// Default limit (limit <= 0) exercises the default path.
	queryVec := make([]float32, 768)
	if _, err := s.Search(ctx, queryVec, 0); err != nil {
		t.Errorf("Search default-limit: %v", err)
	}

	// Flip vecAvailable to false and verify the error path.
	s.vecAvailable = false
	_, err := s.Search(ctx, queryVec, 5)
	if err == nil || !strings.Contains(err.Error(), "vector search unavailable") {
		t.Errorf("expected vector search unavailable error, got %v", err)
	}
}

func TestDeleteDocument_NoChunks(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	// Doc with no chunks at all — Delete should still succeed (idempotent).
	doc := scanner.Document{
		ID: "no-chunks", Path: "/p/n.go", RelPath: "n.go",
		Category: scanner.CategoryUnknown, FileType: scanner.FileTypeCode,
		Content: "x", ContentHash: "n", Metadata: map[string]string{},
	}
	if err := s.UpsertDocument(ctx, doc, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.DeleteDocument(ctx, "no-chunks"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	// Delete an unknown doc — should also succeed (no rows is not an error).
	if err := s.DeleteDocument(ctx, "never-existed"); err != nil {
		t.Errorf("DeleteDocument unknown: %v", err)
	}
}
