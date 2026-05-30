package docs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

// docsStore is an in-memory fake implementation of store.Store sized for the
// docs package tests. Only GetStats, ListCategories and ListFilesByCategory
// carry behaviour; the rest of the Store interface is satisfied with zero
// value stubs in the spirit of internal/brain/fakes_test.go.
type docsStore struct {
	mu sync.Mutex

	stats         store.Stats
	categories    []store.CategorySummary
	filesByCat    map[string][]store.FileSummary
	statsErr      error
	categoriesErr error
	filesErr      error
}

func newDocsStore() *docsStore {
	return &docsStore{
		filesByCat: make(map[string][]store.FileSummary),
	}
}

// --- Document / search surface (stubs) ---

func (d *docsStore) UpsertDocument(_ context.Context, _ scanner.Document, _ []chunker.Chunk) error {
	return nil
}
func (d *docsStore) DeleteDocument(_ context.Context, _ string) error           { return nil }
func (d *docsStore) UpdateScanState(_ context.Context, _ store.ScanState) error { return nil }
func (d *docsStore) Search(_ context.Context, _ []float32, _ int) ([]store.SearchResult, error) {
	return nil, nil
}
func (d *docsStore) SearchFTS(_ context.Context, _ string, _ int) ([]store.SearchResult, error) {
	return nil, nil
}
func (d *docsStore) GetDocument(_ context.Context, _ string) (*scanner.Document, []chunker.Chunk, error) {
	return nil, nil, nil
}
func (d *docsStore) GetScanState(_ context.Context, _ string) (*store.ScanState, error) {
	return nil, nil
}

func (d *docsStore) GetStats(_ context.Context) (store.Stats, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.statsErr != nil {
		return store.Stats{}, d.statsErr
	}
	return d.stats, nil
}

func (d *docsStore) ListCategories(_ context.Context) ([]store.CategorySummary, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.categoriesErr != nil {
		return nil, d.categoriesErr
	}
	out := make([]store.CategorySummary, len(d.categories))
	copy(out, d.categories)
	return out, nil
}

func (d *docsStore) GetContentHash(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (d *docsStore) ListFilesByCategory(_ context.Context, category string, _ int) ([]store.FileSummary, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.filesErr != nil {
		return nil, d.filesErr
	}
	files := d.filesByCat[category]
	out := make([]store.FileSummary, len(files))
	copy(out, files)
	return out, nil
}

func (d *docsStore) ListAllFiles(_ context.Context) ([]store.FileSummary, error) {
	return nil, nil
}

// --- Brain surface (stubs) ---

func (d *docsStore) UpsertBrain(_ context.Context, _ store.Brain) error         { return nil }
func (d *docsStore) GetBrain(_ context.Context, _ string) (*store.Brain, error) { return nil, nil }
func (d *docsStore) ListBrains(_ context.Context, _, _ string, _ int) ([]store.Brain, error) {
	return nil, nil
}
func (d *docsStore) GetBrainStats(_ context.Context) (store.BrainStats, error) {
	return store.BrainStats{}, nil
}

// --- Activity surface (stubs) ---

func (d *docsStore) DeleteBrainActivity(_ context.Context, _ string) error          { return nil }
func (d *docsStore) UpsertActivity(_ context.Context, _ store.HourlyActivity) error { return nil }
func (d *docsStore) ListActivity(_ context.Context, _, _ string) ([]store.HourlyActivity, error) {
	return nil, nil
}

// --- Agent message surface (stubs) ---

func (d *docsStore) UpsertAgentMessage(_ context.Context, _ store.AgentMessage) error {
	return nil
}
func (d *docsStore) SearchAgentMessages(_ context.Context, _, _ string, _ int) ([]store.AgentMessage, error) {
	return nil, nil
}
func (d *docsStore) ListAgentActivity(_ context.Context, _, _, _ string, _ int) ([]store.AgentMessage, error) {
	return nil, nil
}

func (d *docsStore) Close() error { return nil }

// --- Test helpers ---

// seededStore returns a docsStore pre-populated with two categories spanning
// three files in the "go" category (two directories) and one file in the
// "markdown" category. The category counts deliberately differ to exercise
// the FileCount-desc sort order in generateIndex.
func seededStore() *docsStore {
	s := newDocsStore()
	s.stats = store.Stats{
		Repos:      2,
		Files:      4,
		Chunks:     12,
		Categories: 2,
		LastScan:   "2026-05-30T18:30:00Z",
		DBSizeMB:   1.25,
	}
	s.categories = []store.CategorySummary{
		// markdown listed first to ensure the test relies on the
		// FileCount-desc sort to verify ordering, not insertion order.
		{Name: "markdown", FileCount: 1, ChunkCount: 2},
		{Name: "go", FileCount: 3, ChunkCount: 10},
	}
	s.filesByCat["go"] = []store.FileSummary{
		{RelPath: "cmd/cli/main.go", Language: "go", FileType: "source"},
		{RelPath: "internal/web/server.go", Language: "go", FileType: "source"},
		{RelPath: "internal/web/handlers.go", Language: "go", FileType: "source"},
	}
	s.filesByCat["markdown"] = []store.FileSummary{
		{RelPath: "docs/README.md", Language: "", FileType: "doc"},
	}
	return s
}

// readFile reads a file under docsPath, failing the test on error.
func readFile(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("read %v: %v", parts, err)
	}
	return string(b)
}

// --- Tests ---

func TestGenerate_HappyPath(t *testing.T) {
	dir := t.TempDir()
	g := NewGenerator(seededStore(), dir)

	if err := g.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Index file written.
	index := readFile(t, dir, "index.md")
	wantInIndex := []string{
		"# Jor-El Knowledge Base Index",
		"| Repositories | 2 |",
		"| Files indexed | 4 |",
		"| Chunks | 12 |",
		"| Categories | 2 |",
		"| DB size | 1.25 MB |",
		"| Last scan | 2026-05-30T18:30:00Z |",
		"## Categories",
		"[go](categories/go.md) — 3 files",
		"[markdown](categories/markdown.md) — 1 files",
	}
	for _, want := range wantInIndex {
		if !strings.Contains(index, want) {
			t.Errorf("index.md missing %q\n--- index.md ---\n%s", want, index)
		}
	}

	// FileCount-desc sort: "go" (3 files) must appear before "markdown".
	if pgo, pmd := strings.Index(index, "categories/go.md"), strings.Index(index, "categories/markdown.md"); pgo == -1 || pmd == -1 || pgo > pmd {
		t.Errorf("expected go category listed before markdown in FileCount-desc order; got go@%d markdown@%d", pgo, pmd)
	}

	// Per-category go file: header, count, grouped dirs, language tag.
	goCat := readFile(t, dir, "categories", "go.md")
	wantInGo := []string{
		"# Category: go",
		"*3 files, 10 chunks*",
		"## Files",
		"### `cmd/cli/`",
		"- `main.go` (go)",
		"### `internal/web/`",
		"- `server.go` (go)",
		"- `handlers.go` (go)",
	}
	for _, want := range wantInGo {
		if !strings.Contains(goCat, want) {
			t.Errorf("categories/go.md missing %q\n--- go.md ---\n%s", want, goCat)
		}
	}

	// Directories should appear in alphabetical order: cmd/cli before internal/web.
	if pCmd, pInt := strings.Index(goCat, "### `cmd/cli/`"), strings.Index(goCat, "### `internal/web/`"); pCmd == -1 || pInt == -1 || pCmd > pInt {
		t.Errorf("expected cmd/cli/ before internal/web/ in alphabetical order; got cmd@%d internal@%d", pCmd, pInt)
	}

	// Per-category markdown file: empty language tag should not render parens.
	mdCat := readFile(t, dir, "categories", "markdown.md")
	if !strings.Contains(mdCat, "# Category: markdown") {
		t.Errorf("categories/markdown.md missing header\n%s", mdCat)
	}
	if !strings.Contains(mdCat, "- `README.md`\n") {
		t.Errorf("expected README.md entry with no language suffix\n%s", mdCat)
	}
	if strings.Contains(mdCat, "README.md (") {
		t.Errorf("README.md should not have a language suffix when Language is empty\n%s", mdCat)
	}
}

func TestGenerate_CreatesCategoriesSubdir(t *testing.T) {
	dir := t.TempDir()
	g := NewGenerator(newDocsStore(), dir)

	if err := g.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "categories"))
	if err != nil {
		t.Fatalf("stat categories/: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected categories/ to be a directory")
	}
}

func TestGenerate_EmptyCategories(t *testing.T) {
	dir := t.TempDir()
	s := newDocsStore() // no categories, no files
	s.stats = store.Stats{Repos: 0, Files: 0, Chunks: 0, Categories: 0}

	g := NewGenerator(s, dir)
	if err := g.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	index := readFile(t, dir, "index.md")
	if !strings.Contains(index, "## Categories") {
		t.Errorf("expected empty index to still include Categories header\n%s", index)
	}

	// No per-category files should be written.
	entries, err := os.ReadDir(filepath.Join(dir, "categories"))
	if err != nil {
		t.Fatalf("readdir categories/: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected empty categories/ directory; got %v", names)
	}
}

func TestGenerate_NoLastScanOmitsRow(t *testing.T) {
	dir := t.TempDir()
	s := newDocsStore()
	s.stats = store.Stats{Repos: 1, Files: 1, Chunks: 1, Categories: 0, LastScan: ""}

	g := NewGenerator(s, dir)
	if err := g.Generate(context.Background()); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	index := readFile(t, dir, "index.md")
	if strings.Contains(index, "| Last scan |") {
		t.Errorf("expected Last scan row to be omitted when LastScan is empty\n%s", index)
	}
}

func TestGenerate_StoreErrors(t *testing.T) {
	t.Run("stats error surfaces", func(t *testing.T) {
		dir := t.TempDir()
		s := seededStore()
		s.statsErr = errors.New("stats boom")
		g := NewGenerator(s, dir)

		err := g.Generate(context.Background())
		if err == nil || !strings.Contains(err.Error(), "getting stats") {
			t.Fatalf("expected getting stats error, got %v", err)
		}
	})

	t.Run("categories error surfaces", func(t *testing.T) {
		dir := t.TempDir()
		s := seededStore()
		s.categoriesErr = errors.New("cat boom")
		g := NewGenerator(s, dir)

		err := g.Generate(context.Background())
		if err == nil || !strings.Contains(err.Error(), "listing categories") {
			t.Fatalf("expected listing categories error, got %v", err)
		}
	})

	t.Run("file list error is tolerated and skipped silently", func(t *testing.T) {
		dir := t.TempDir()
		s := seededStore()
		s.filesErr = errors.New("files boom")
		g := NewGenerator(s, dir)

		if err := g.Generate(context.Background()); err != nil {
			t.Fatalf("Generate should tolerate ListFilesByCategory errors: %v", err)
		}

		// Category file should still be written but without a Files section,
		// since generateCategory skips the listing block when err != nil.
		goCat := readFile(t, dir, "categories", "go.md")
		if !strings.Contains(goCat, "# Category: go") {
			t.Errorf("expected category header even when file list errored\n%s", goCat)
		}
		if strings.Contains(goCat, "## Files") {
			t.Errorf("expected no Files section when ListFilesByCategory errors\n%s", goCat)
		}
	})
}

func TestGenerate_MkdirFails(t *testing.T) {
	// Create a regular file where docsPath/categories needs to live, so the
	// MkdirAll(docsPath/categories, ...) call fails with ENOTDIR.
	parent := t.TempDir()
	docsPath := filepath.Join(parent, "docs")
	// docsPath itself is a regular file: MkdirAll(docsPath/categories) will
	// fail because docsPath is not a directory.
	if err := os.WriteFile(docsPath, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	g := NewGenerator(newDocsStore(), docsPath)
	err := g.Generate(context.Background())
	if err == nil || !strings.Contains(err.Error(), "creating docs directory") {
		t.Fatalf("expected creating docs directory error, got %v", err)
	}
}
