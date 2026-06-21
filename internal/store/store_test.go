package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
)

func testDB(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := New(dbPath, 768)
	if err != nil {
		t.Fatalf("creating test DB: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertAndGetDocument(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	doc := scanner.Document{
		ID:          "doc1",
		Path:        "/tmp/test/main.go",
		RelPath:     "main.go",
		Category:    scanner.CategoryUnknown,
		Language:    "go",
		FileType:    scanner.FileTypeCode,
		Content:     "package main\n\nfunc main() {}\n",
		ContentHash: "abc123",
		Metadata:    map[string]string{},
	}

	chunks := []chunker.Chunk{
		{
			ID:         "chunk1",
			DocumentID: "doc1",
			Content:    "package main\n\nfunc main() {}\n",
			StartLine:  1,
			EndLine:    3,
			Metadata: chunker.ChunkMeta{
				Path:     "main.go",
				Language: "go",
				FileType: scanner.FileTypeCode,
			},
		},
	}

	err := s.UpsertDocument(ctx, doc, chunks)
	if err != nil {
		t.Fatalf("UpsertDocument: %v", err)
	}

	got, gotChunks, err := s.GetDocument(ctx, "main.go")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}

	if got.ID != "doc1" {
		t.Errorf("expected doc ID doc1, got %s", got.ID)
	}
	if got.Language != "go" {
		t.Errorf("expected language go, got %s", got.Language)
	}
	if len(gotChunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(gotChunks))
	}
	if gotChunks[0].Content != "package main\n\nfunc main() {}\n" {
		t.Errorf("unexpected chunk content: %q", gotChunks[0].Content)
	}
}

func TestDeleteDocument(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	doc := scanner.Document{
		ID:          "doc2",
		Path:        "/tmp/test/old.go",
		RelPath:     "old.go",
		Category:    scanner.CategoryUnknown,
		FileType:    scanner.FileTypeCode,
		Content:     "package old",
		ContentHash: "def456",
		Metadata:    map[string]string{},
	}

	chunks := []chunker.Chunk{
		{ID: "chunk2", DocumentID: "doc2", Content: "package old", StartLine: 1, EndLine: 1},
	}

	s.UpsertDocument(ctx, doc, chunks)
	err := s.DeleteDocument(ctx, "doc2")
	if err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}

	_, _, err = s.GetDocument(ctx, "old.go")
	if err == nil {
		t.Error("expected error getting deleted document")
	}
}

func TestGetStats(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	doc := scanner.Document{
		ID: "doc3", Path: "/tmp/test/a.go", RelPath: "a.go",
		Category: scanner.CategoryAPI, FileType: scanner.FileTypeCode,
		Content: "package a", ContentHash: "hash1", Metadata: map[string]string{},
	}
	chunks := []chunker.Chunk{
		{ID: "c1", DocumentID: "doc3", Content: "package a", StartLine: 1, EndLine: 1},
	}
	s.UpsertDocument(ctx, doc, chunks)

	stats, err := s.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}

	if stats.Files != 1 {
		t.Errorf("expected 1 file, got %d", stats.Files)
	}
	if stats.Chunks != 1 {
		t.Errorf("expected 1 chunk, got %d", stats.Chunks)
	}
}

func TestGetContentHash(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	doc := scanner.Document{
		ID: "doc4", Path: "/tmp/test/b.go", RelPath: "b.go",
		Category: scanner.CategoryUnknown, FileType: scanner.FileTypeCode,
		Content: "content", ContentHash: "myhash", Metadata: map[string]string{},
	}
	s.UpsertDocument(ctx, doc, nil)

	hash, err := s.GetContentHash(ctx, "doc4")
	if err != nil {
		t.Fatalf("GetContentHash: %v", err)
	}
	if hash != "myhash" {
		t.Errorf("expected hash myhash, got %s", hash)
	}

	hash, _ = s.GetContentHash(ctx, "nonexistent")
	if hash != "" {
		t.Errorf("expected empty hash for nonexistent doc, got %s", hash)
	}
}

func TestListCategories(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	for i, cat := range []scanner.Category{scanner.CategoryAPI, scanner.CategoryAPI, scanner.CategoryDocs} {
		doc := scanner.Document{
			ID: "cat-doc-" + itoa(i), Path: "/tmp/" + itoa(i), RelPath: itoa(i) + ".go",
			Category: cat, FileType: scanner.FileTypeCode,
			Content: "pkg", ContentHash: "h" + itoa(i), Metadata: map[string]string{},
		}
		s.UpsertDocument(ctx, doc, nil)
	}

	cats, err := s.ListCategories(ctx)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}

	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(cats))
	}
}

func TestSearchFTS(t *testing.T) {
	s := testDB(t)
	if !s.ftsAvailable {
		t.Skip("FTS5 not built in; run: make test (uses -tags sqlite_fts5)")
	}
	ctx := context.Background()

	doc := scanner.Document{
		ID: "fts-doc", Path: "/tmp/fts.go", RelPath: "fts.go",
		Category: scanner.CategoryUnknown, FileType: scanner.FileTypeCode,
		Content: "package auth\nfunc Login() {}", ContentHash: "ftshash",
		Metadata: map[string]string{},
	}
	chunks := []chunker.Chunk{
		{ID: "fts-c1", DocumentID: "fts-doc", Content: "package auth\nfunc Login() {}", StartLine: 1, EndLine: 2},
	}
	s.UpsertDocument(ctx, doc, chunks)

	results, err := s.SearchFTS(ctx, "Login", 5)
	if err != nil {
		t.Fatalf("SearchFTS: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one FTS result")
	}
}

func TestVectorSearch(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	if !s.vecAvailable {
		t.Skip("sqlite-vec extension not available")
	}

	// Create a document with a chunk that has an embedding
	doc := scanner.Document{
		ID: "vec-doc", Path: "/tmp/vec.go", RelPath: "vec.go",
		Category: scanner.CategoryAPI, FileType: scanner.FileTypeCode,
		Content: "package api\nfunc HandleRequest() {}", ContentHash: "vechash",
		Metadata: map[string]string{},
	}

	embedding := make([]float32, 768)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}

	chunks := []chunker.Chunk{
		{
			ID: "vec-c1", DocumentID: "vec-doc",
			Content:   "package api\nfunc HandleRequest() {}",
			StartLine: 1, EndLine: 2,
			Embedding: embedding,
			Metadata: chunker.ChunkMeta{
				Path: "vec.go", Category: scanner.CategoryAPI,
				Language: "go", FileType: scanner.FileTypeCode,
			},
		},
	}

	err := s.UpsertDocument(ctx, doc, chunks)
	if err != nil {
		t.Fatalf("UpsertDocument with embedding: %v", err)
	}

	// Search with a similar vector
	queryVec := make([]float32, 768)
	for i := range queryVec {
		queryVec[i] = float32(i) * 0.001
	}

	results, err := s.Search(ctx, queryVec, 5)
	if err != nil {
		t.Fatalf("Vector search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one vector search result")
	}

	if results[0].Chunk.Metadata.Path != "vec.go" {
		t.Errorf("expected result path vec.go, got %s", results[0].Chunk.Metadata.Path)
	}

	// Score should be very high (near 1.0) since query = document
	if results[0].Score < 0.99 {
		t.Errorf("expected score near 1.0 for identical vectors, got %f", results[0].Score)
	}
}

// --- agentops-111: extend Store.GetDocument and Store.Search coverage ---
// Fills paths left after TestUpsertAndGetDocument / TestVectorSearch /
// TestSearchFTS and the closed-DB error wraps in query_test.go: GetDocument
// not-found and absolute-path branches, metadata round-trip with multi-chunk
// ordering, and Search distance ranking / limit truncation / empty-index.

// TestGetDocument_NotFound asserts a lookup of a never-indexed path returns an
// error and a nil doc even when the DB already holds other documents.
func TestGetDocument_NotFound(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	seed := scanner.Document{
		ID: "nf-seed", Path: "/tmp/seed.go", RelPath: "seed.go",
		Category: scanner.CategoryUnknown, FileType: scanner.FileTypeCode,
		Content: "package seed", ContentHash: "nfhash", Metadata: map[string]string{},
	}
	if err := s.UpsertDocument(ctx, seed, nil); err != nil {
		t.Fatalf("seed UpsertDocument: %v", err)
	}

	doc, chunks, err := s.GetDocument(ctx, "does/not/exist.go")
	if err == nil {
		t.Fatal("expected error for unknown path, got nil")
	}
	if doc != nil {
		t.Errorf("expected nil doc on not-found, got %+v", doc)
	}
	if chunks != nil {
		t.Errorf("expected nil chunks on not-found, got %d", len(chunks))
	}
}

// TestGetDocument_ByAbsolutePath exercises the OR path branch: a doc seeded with
// a RelPath distinct from its absolute Path resolves when looked up by Path.
func TestGetDocument_ByAbsolutePath(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	doc := scanner.Document{
		ID: "abs-1", Path: "/abs/root/pkg/server.go", RelPath: "pkg/server.go",
		Category: scanner.CategoryAPI, Language: "go", FileType: scanner.FileTypeCode,
		Content: "package pkg", ContentHash: "abshash", Metadata: map[string]string{},
	}
	if err := s.UpsertDocument(ctx, doc, nil); err != nil {
		t.Fatalf("UpsertDocument: %v", err)
	}

	got, _, err := s.GetDocument(ctx, "/abs/root/pkg/server.go")
	if err != nil {
		t.Fatalf("GetDocument by absolute path: %v", err)
	}
	if got.ID != "abs-1" {
		t.Errorf("expected doc abs-1 via absolute path, got %s", got.ID)
	}
	if got.RelPath != "pkg/server.go" {
		t.Errorf("expected rel_path pkg/server.go, got %s", got.RelPath)
	}
}

// TestGetDocument_MetadataAndChunkOrder asserts metadata JSON round-trips, chunks
// come back ordered by start_line (seeded out of order), and each chunk ChunkMeta
// is populated from the parent document.
func TestGetDocument_MetadataAndChunkOrder(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()

	doc := scanner.Document{
		ID: "meta-1", Repo: "cerebra", RepoRoot: "/repos/cerebra",
		Path: "/repos/cerebra/x.go", RelPath: "x.go",
		Category: scanner.CategoryAPI, Language: "go", FileType: scanner.FileTypeCode,
		Content: "package x", ContentHash: "metahash",
		Metadata: map[string]string{"author": "gopher", "remote_url": "https://example/cerebra"},
	}
	// Seed chunks out of start_line order to prove ORDER BY start_line.
	chunks := []chunker.Chunk{
		{ID: "meta-c2", DocumentID: "meta-1", Content: "second", StartLine: 10, EndLine: 12},
		{ID: "meta-c1", DocumentID: "meta-1", Content: "first", StartLine: 1, EndLine: 3},
	}
	if err := s.UpsertDocument(ctx, doc, chunks); err != nil {
		t.Fatalf("UpsertDocument: %v", err)
	}

	got, gotChunks, err := s.GetDocument(ctx, "x.go")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if got.Repo != "cerebra" {
		t.Errorf("expected repo cerebra, got %q", got.Repo)
	}
	if got.Metadata["author"] != "gopher" {
		t.Errorf("expected metadata author=gopher, got %q", got.Metadata["author"])
	}
	if len(gotChunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(gotChunks))
	}
	if gotChunks[0].ID != "meta-c1" || gotChunks[1].ID != "meta-c2" {
		t.Errorf("expected chunks ordered by start_line (meta-c1, meta-c2), got %s, %s",
			gotChunks[0].ID, gotChunks[1].ID)
	}
	if gotChunks[0].Metadata.Repo != "cerebra" {
		t.Errorf("expected chunk ChunkMeta.Repo cerebra, got %q", gotChunks[0].Metadata.Repo)
	}
	if gotChunks[0].Metadata.Path != "x.go" {
		t.Errorf("expected chunk ChunkMeta.Path x.go, got %q", gotChunks[0].Metadata.Path)
	}
	if gotChunks[0].Metadata.Category != scanner.CategoryAPI {
		t.Errorf("expected chunk ChunkMeta.Category api, got %q", gotChunks[0].Metadata.Category)
	}
}

// TestSearch_RankingAndLimit seeds three chunks at increasing distance from the
// query and asserts closest-first ordering (descending Score) and limit truncation.
func TestSearch_RankingAndLimit(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	if !s.vecAvailable {
		t.Skip("sqlite-vec extension not available")
	}

	const dim = 768
	mkEmbedding := func(v float32) []float32 {
		e := make([]float32, dim)
		for i := range e {
			e[i] = v
		}
		return e
	}

	// near=0.0, mid=0.5, far=1.0; query is all-zeros so near is closest.
	seed := []struct {
		id  string
		val float32
	}{
		{"rank-far", 1.0},
		{"rank-near", 0.0},
		{"rank-mid", 0.5},
	}
	for _, sd := range seed {
		doc := scanner.Document{
			ID: sd.id, Path: "/tmp/" + sd.id + ".go", RelPath: sd.id + ".go",
			Category: scanner.CategoryAPI, Language: "go", FileType: scanner.FileTypeCode,
			Content: "package r", ContentHash: "h-" + sd.id, Metadata: map[string]string{},
		}
		chunks := []chunker.Chunk{
			{
				ID: sd.id + "-c", DocumentID: sd.id, Content: "package r",
				StartLine: 1, EndLine: 1, Embedding: mkEmbedding(sd.val),
			},
		}
		if err := s.UpsertDocument(ctx, doc, chunks); err != nil {
			t.Fatalf("UpsertDocument %s: %v", sd.id, err)
		}
	}

	query := make([]float32, dim) // all zeros

	// limit=3 -> all three, ranked closest-first.
	results, err := s.Search(ctx, query, 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Chunk.ID != "rank-near-c" {
		t.Errorf("expected closest chunk first (rank-near-c), got %s", results[0].Chunk.ID)
	}
	for i := 1; i < len(results); i++ {
		if results[i-1].Score < results[i].Score {
			t.Errorf("expected scores in descending order, got %f before %f",
				results[i-1].Score, results[i].Score)
		}
	}

	// limit=1 -> truncated to the single closest result.
	top, err := s.Search(ctx, query, 1)
	if err != nil {
		t.Fatalf("Search limit=1: %v", err)
	}
	if len(top) != 1 {
		t.Fatalf("expected 1 result with limit=1, got %d", len(top))
	}
	if top[0].Chunk.ID != "rank-near-c" {
		t.Errorf("expected rank-near-c as top result, got %s", top[0].Chunk.ID)
	}
}

// TestSearch_EmptyIndex asserts a search on a fresh DB returns an empty slice and
// no error, protecting the MCP search surface on a cold index.
func TestSearch_EmptyIndex(t *testing.T) {
	s := testDB(t)
	ctx := context.Background()
	if !s.vecAvailable {
		t.Skip("sqlite-vec extension not available")
	}

	results, err := s.Search(ctx, make([]float32, 768), 5)
	if err != nil {
		t.Fatalf("Search on empty index: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results on empty index, got %d", len(results))
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
