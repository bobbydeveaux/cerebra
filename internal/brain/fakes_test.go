package brain

// fakes_test.go provides in-memory test doubles for the dependencies that
// internal/brain reaches into: store.Store and embedder.Embedder. The brain
// package never calls the document-search surface of the store, but the Store
// interface is wide so we satisfy it with zero-value stubs and only seed the
// fields exercised by the brain code paths (brains, activity, agent messages,
// indexed documents, content hashes).

import (
	"context"
	"sync"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

// fakeStore implements store.Store with pre-seeded in-memory data.
type fakeStore struct {
	mu sync.Mutex

	// Document surface (only the bits brain.Indexer touches).
	contentHashes map[string]string                 // docID -> hash
	upsertedDocs  map[string]scanner.Document       // docID -> doc
	upsertedChunks map[string][]chunker.Chunk       // docID -> chunks

	// Brain surface.
	brains map[string]*store.Brain // brainID -> brain (pointer so tests can mutate)

	// Activity surface.
	activity        []store.HourlyActivity
	activityDeleted []string // brainIDs whose activity was cleared

	// Agent message surface.
	agentMessages []store.AgentMessage

	// Error toggles for failure-path coverage.
	upsertBrainErr     error
	upsertActivityErr  error
	upsertAgentMsgErr  error
	upsertDocumentErr  error
	listBrainsErr      error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		contentHashes:  make(map[string]string),
		upsertedDocs:   make(map[string]scanner.Document),
		upsertedChunks: make(map[string][]chunker.Chunk),
		brains:         make(map[string]*store.Brain),
	}
}

// --- Document / search surface (mostly stubs for brain) ---

func (f *fakeStore) UpsertDocument(_ context.Context, doc scanner.Document, chunks []chunker.Chunk) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upsertDocumentErr != nil {
		return f.upsertDocumentErr
	}
	f.upsertedDocs[doc.ID] = doc
	f.upsertedChunks[doc.ID] = chunks
	f.contentHashes[doc.ID] = doc.ContentHash
	return nil
}

func (f *fakeStore) DeleteDocument(_ context.Context, _ string) error          { return nil }
func (f *fakeStore) UpdateScanState(_ context.Context, _ store.ScanState) error { return nil }

func (f *fakeStore) Search(_ context.Context, _ []float32, _ int) ([]store.SearchResult, error) {
	return nil, nil
}
func (f *fakeStore) SearchFTS(_ context.Context, _ string, _ int) ([]store.SearchResult, error) {
	return nil, nil
}
func (f *fakeStore) GetDocument(_ context.Context, _ string) (*scanner.Document, []chunker.Chunk, error) {
	return nil, nil, nil
}
func (f *fakeStore) GetScanState(_ context.Context, _ string) (*store.ScanState, error) {
	return nil, nil
}
func (f *fakeStore) GetStats(_ context.Context) (store.Stats, error) {
	return store.Stats{}, nil
}
func (f *fakeStore) ListCategories(_ context.Context) ([]store.CategorySummary, error) {
	return nil, nil
}

func (f *fakeStore) GetContentHash(_ context.Context, docID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.contentHashes[docID], nil
}

func (f *fakeStore) ListFilesByCategory(_ context.Context, _ string, _ int) ([]store.FileSummary, error) {
	return nil, nil
}
func (f *fakeStore) ListAllFiles(_ context.Context) ([]store.FileSummary, error) {
	return nil, nil
}

// --- Brain surface ---

func (f *fakeStore) UpsertBrain(_ context.Context, b store.Brain) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upsertBrainErr != nil {
		return f.upsertBrainErr
	}
	clone := b
	f.brains[b.BrainID] = &clone
	return nil
}

func (f *fakeStore) GetBrain(_ context.Context, brainID string) (*store.Brain, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.brains[brainID]; ok {
		clone := *b
		return &clone, nil
	}
	return nil, nil
}

func (f *fakeStore) ListBrains(_ context.Context, projectKey string, status string, _ int) ([]store.Brain, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listBrainsErr != nil {
		return nil, f.listBrainsErr
	}
	out := make([]store.Brain, 0, len(f.brains))
	for _, b := range f.brains {
		if projectKey != "" && b.ProjectKey != projectKey {
			continue
		}
		if status != "" && b.Status != status {
			continue
		}
		out = append(out, *b)
	}
	return out, nil
}

func (f *fakeStore) GetBrainStats(_ context.Context) (store.BrainStats, error) {
	return store.BrainStats{}, nil
}

// --- Activity surface ---

func (f *fakeStore) DeleteBrainActivity(_ context.Context, brainID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.activityDeleted = append(f.activityDeleted, brainID)
	return nil
}

func (f *fakeStore) UpsertActivity(_ context.Context, a store.HourlyActivity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upsertActivityErr != nil {
		return f.upsertActivityErr
	}
	f.activity = append(f.activity, a)
	return nil
}

func (f *fakeStore) ListActivity(_ context.Context, _ string, _ string) ([]store.HourlyActivity, error) {
	return nil, nil
}

// --- Agent message surface ---

func (f *fakeStore) UpsertAgentMessage(_ context.Context, m store.AgentMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upsertAgentMsgErr != nil {
		return f.upsertAgentMsgErr
	}
	f.agentMessages = append(f.agentMessages, m)
	return nil
}

func (f *fakeStore) SearchAgentMessages(_ context.Context, _ string, _ string, _ int) ([]store.AgentMessage, error) {
	return nil, nil
}

func (f *fakeStore) ListAgentActivity(_ context.Context, _ string, _ string, _ string, _ int) ([]store.AgentMessage, error) {
	return nil, nil
}

func (f *fakeStore) Close() error { return nil }

// fakeEmbedder returns a deterministic per-call vector. Useful for
// exercising the Indexer happy-path without spinning up Ollama or OpenAI.
type fakeEmbedder struct {
	vec      []float32
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
	if len(e.vec) == 0 {
		return 4
	}
	return len(e.vec)
}
