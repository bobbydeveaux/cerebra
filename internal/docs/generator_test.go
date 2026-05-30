package docs

import (
	"context"
	"sync"

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
func (d *docsStore) DeleteDocument(_ context.Context, _ string) error              { return nil }
func (d *docsStore) UpdateScanState(_ context.Context, _ store.ScanState) error    { return nil }
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

func (d *docsStore) UpsertBrain(_ context.Context, _ store.Brain) error            { return nil }
func (d *docsStore) GetBrain(_ context.Context, _ string) (*store.Brain, error)    { return nil, nil }
func (d *docsStore) ListBrains(_ context.Context, _, _ string, _ int) ([]store.Brain, error) {
	return nil, nil
}
func (d *docsStore) GetBrainStats(_ context.Context) (store.BrainStats, error) {
	return store.BrainStats{}, nil
}

// --- Activity surface (stubs) ---

func (d *docsStore) DeleteBrainActivity(_ context.Context, _ string) error      { return nil }
func (d *docsStore) UpsertActivity(_ context.Context, _ store.HourlyActivity) error {
	return nil
}
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
