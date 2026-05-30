package web

// fakes_test.go provides in-memory test doubles for the dependencies that
// internal/web's HTTP handlers reach into: store.Store and embedder.Embedder.
// The handlers are thin, so a fake whose methods return pre-seeded slices is
// enough to exercise the rendering paths without spinning up a real SQLite
// database, the embedder backend, or the RAG pipeline.

import (
	"context"
	"errors"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

// fakeStore implements store.Store with pre-seeded in-memory data. Unused
// pipeline methods return zero values or nil errors so the surface stays
// minimal for the handler tests.
type fakeStore struct {
	stats         store.Stats
	categories    []store.CategorySummary
	files         []store.FileSummary
	filesByCat    map[string][]store.FileSummary
	docs          map[string]*scanner.Document
	docChunks     map[string][]chunker.Chunk
	searchResults []store.SearchResult
	ftsResults    []store.SearchResult
	brains        []store.Brain
	brainsByID    map[string]*store.Brain
	brainStats    store.BrainStats

	// Toggle errors for failure-path coverage.
	getStatsErr      error
	listBrainsErr    error
	getBrainStatsErr error
	getBrainErr      error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		filesByCat: make(map[string][]store.FileSummary),
		docs:       make(map[string]*scanner.Document),
		docChunks:  make(map[string][]chunker.Chunk),
		brainsByID: make(map[string]*store.Brain),
	}
}

// --- Document / search surface ---

func (f *fakeStore) UpsertDocument(_ context.Context, _ scanner.Document, _ []chunker.Chunk) error {
	return nil
}
func (f *fakeStore) DeleteDocument(_ context.Context, _ string) error        { return nil }
func (f *fakeStore) UpdateScanState(_ context.Context, _ store.ScanState) error { return nil }

func (f *fakeStore) Search(_ context.Context, _ []float32, _ int) ([]store.SearchResult, error) {
	return f.searchResults, nil
}

func (f *fakeStore) SearchFTS(_ context.Context, _ string, _ int) ([]store.SearchResult, error) {
	return f.ftsResults, nil
}

func (f *fakeStore) GetDocument(_ context.Context, path string) (*scanner.Document, []chunker.Chunk, error) {
	if doc, ok := f.docs[path]; ok {
		return doc, f.docChunks[path], nil
	}
	return nil, nil, errors.New("not found")
}

func (f *fakeStore) GetScanState(_ context.Context, _ string) (*store.ScanState, error) {
	return nil, nil
}

func (f *fakeStore) GetStats(_ context.Context) (store.Stats, error) {
	if f.getStatsErr != nil {
		return store.Stats{}, f.getStatsErr
	}
	return f.stats, nil
}

func (f *fakeStore) ListCategories(_ context.Context) ([]store.CategorySummary, error) {
	return f.categories, nil
}

func (f *fakeStore) GetContentHash(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (f *fakeStore) ListFilesByCategory(_ context.Context, category string, _ int) ([]store.FileSummary, error) {
	return f.filesByCat[category], nil
}

func (f *fakeStore) ListAllFiles(_ context.Context) ([]store.FileSummary, error) {
	return f.files, nil
}

// --- Brain surface ---

func (f *fakeStore) UpsertBrain(_ context.Context, _ store.Brain) error { return nil }

func (f *fakeStore) GetBrain(_ context.Context, brainID string) (*store.Brain, error) {
	if f.getBrainErr != nil {
		return nil, f.getBrainErr
	}
	if b, ok := f.brainsByID[brainID]; ok {
		return b, nil
	}
	return nil, nil
}

func (f *fakeStore) ListBrains(_ context.Context, _ string, _ string, _ int) ([]store.Brain, error) {
	if f.listBrainsErr != nil {
		return nil, f.listBrainsErr
	}
	return f.brains, nil
}

func (f *fakeStore) GetBrainStats(_ context.Context) (store.BrainStats, error) {
	if f.getBrainStatsErr != nil {
		return store.BrainStats{}, f.getBrainStatsErr
	}
	return f.brainStats, nil
}

// --- Activity surface ---

func (f *fakeStore) DeleteBrainActivity(_ context.Context, _ string) error { return nil }
func (f *fakeStore) UpsertActivity(_ context.Context, _ store.HourlyActivity) error {
	return nil
}
func (f *fakeStore) ListActivity(_ context.Context, _ string, _ string) ([]store.HourlyActivity, error) {
	return nil, nil
}

// --- Agent message surface ---

func (f *fakeStore) UpsertAgentMessage(_ context.Context, _ store.AgentMessage) error {
	return nil
}
func (f *fakeStore) SearchAgentMessages(_ context.Context, _ string, _ string, _ int) ([]store.AgentMessage, error) {
	return nil, nil
}
func (f *fakeStore) ListAgentActivity(_ context.Context, _ string, _ string, _ string, _ int) ([]store.AgentMessage, error) {
	return nil, nil
}

func (f *fakeStore) Close() error { return nil }

// fakeEmbedder implements embedder.Embedder by returning a pre-set vector
// per call. Useful for exercising handleSearchAPI's vector-then-FTS fallback.
type fakeEmbedder struct {
	dim     int
	vec     []float32
	embedErr error
}

func (e *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if e.embedErr != nil {
		return nil, e.embedErr
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = e.vec
	}
	return out, nil
}

func (e *fakeEmbedder) Dimensions() int {
	if e.dim == 0 {
		return 4
	}
	return e.dim
}

// fakeChatPipeline implements the chatPipeline interface used by
// handleChatStream. It returns a pre-canned error (when answerErr is set)
// or streams pre-canned tokens (from tokens) through a buffered channel.
// The recorded question and history let tests assert that the handler
// forwards the URL query params unchanged.
type fakeChatPipeline struct {
	tokens    []string
	answerErr error

	// Captured by AnswerWithHistory for assertion.
	gotQuestion string
	gotHistory  []map[string]string
}

func (f *fakeChatPipeline) AnswerWithHistory(_ context.Context, question string, history []map[string]string) (<-chan string, error) {
	f.gotQuestion = question
	f.gotHistory = history

	if f.answerErr != nil {
		return nil, f.answerErr
	}

	ch := make(chan string, len(f.tokens))
	for _, tok := range f.tokens {
		ch <- tok
	}
	close(ch)
	return ch, nil
}
