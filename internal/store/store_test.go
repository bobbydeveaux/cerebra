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
			Content: "package api\nfunc HandleRequest() {}",
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

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
