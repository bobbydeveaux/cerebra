package embedder

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
)

type mockEmbedder struct {
	dim int
}

func (m *mockEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i := range texts {
		vec := make([]float32, m.dim)
		for j := range vec {
			vec[j] = float32(i) * 0.1
		}
		results[i] = vec
	}
	return results, nil
}

func (m *mockEmbedder) Dimensions() int {
	return m.dim
}

// errEmbedder always returns the configured error from Embed. Used to exercise
// the error-propagation branch in Pool.EmbedChunks.
type errEmbedder struct {
	dim int
	err error
}

func (e *errEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, e.err
}

func (e *errEmbedder) Dimensions() int { return e.dim }

// slowEmbedder blocks on the context, releasing only when the context is
// cancelled or the configured delay elapses. Used to exercise the
// context-cancel branch in Pool.EmbedChunks.
type slowEmbedder struct {
	dim   int
	delay time.Duration
}

func (s *slowEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	results := make([][]float32, len(texts))
	for i := range results {
		results[i] = make([]float32, s.dim)
	}
	return results, nil
}

func (s *slowEmbedder) Dimensions() int { return s.dim }

func TestPool_EmbedChunks(t *testing.T) {
	emb := &mockEmbedder{dim: 768}
	pool := NewPool(emb, 2, 4)

	chunks := make([]chunker.Chunk, 10)
	for i := range chunks {
		chunks[i] = chunker.Chunk{
			ID:         "chunk-" + string(rune('a'+i)),
			DocumentID: "doc1",
			Content:    "test content " + string(rune('a'+i)),
			Metadata: chunker.ChunkMeta{
				Path:     "test.go",
				FileType: scanner.FileTypeCode,
			},
		}
	}

	var progressCount int
	result, err := pool.EmbedChunks(context.Background(), chunks, func(n int) {
		progressCount += n
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 10 {
		t.Fatalf("expected 10 chunks, got %d", len(result))
	}

	for i, c := range result {
		if c.Embedding == nil {
			t.Errorf("chunk %d has nil embedding", i)
		}
		if len(c.Embedding) != 768 {
			t.Errorf("chunk %d embedding length = %d, want 768", i, len(c.Embedding))
		}
	}

	if progressCount != 10 {
		t.Errorf("progress count = %d, want 10", progressCount)
	}
}

func TestPool_EmptyChunks(t *testing.T) {
	emb := &mockEmbedder{dim: 768}
	pool := NewPool(emb, 2, 4)

	result, err := pool.EmbedChunks(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for nil input, got %v", result)
	}
}

func TestNewPool_DefaultsWorkersAndBatchSize(t *testing.T) {
	emb := &mockEmbedder{dim: 4}
	p := NewPool(emb, 0, 0)
	if p == nil {
		t.Fatal("NewPool returned nil")
	}
	if p.workers <= 0 {
		t.Errorf("workers = %d, want > 0", p.workers)
	}
	if p.workers > 4 {
		t.Errorf("workers = %d, want capped at 4", p.workers)
	}
	if p.batchSize != 32 {
		t.Errorf("batchSize = %d, want 32 (default)", p.batchSize)
	}
}

func TestNewPool_ExplicitConfig(t *testing.T) {
	emb := &mockEmbedder{dim: 4}
	p := NewPool(emb, 3, 7)
	if p.workers != 3 {
		t.Errorf("workers = %d, want 3", p.workers)
	}
	if p.batchSize != 7 {
		t.Errorf("batchSize = %d, want 7", p.batchSize)
	}
}

func TestPool_EmbedChunks_EmbedderError(t *testing.T) {
	wantErr := errors.New("upstream embedding failed")
	emb := &errEmbedder{dim: 8, err: wantErr}
	pool := NewPool(emb, 2, 2)

	chunks := make([]chunker.Chunk, 4)
	for i := range chunks {
		chunks[i] = chunker.Chunk{
			ID:      "c",
			Content: "blob",
			Metadata: chunker.ChunkMeta{
				Path:     "x.go",
				FileType: scanner.FileTypeCode,
			},
		}
	}

	_, err := pool.EmbedChunks(context.Background(), chunks, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "embedding batch") {
		t.Errorf("error = %v, want wrapped 'embedding batch'", err)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want errors.Is(%v) to be true", err, wantErr)
	}
}

func TestPool_EmbedChunks_TruncatesLongContent(t *testing.T) {
	// Build a chunk longer than the 2000-char truncation threshold and
	// verify the embedder receives the truncated form.
	long := strings.Repeat("x", 2500)
	captured := make(chan string, 1)
	emb := &captureEmbedder{dim: 4, captured: captured}

	pool := NewPool(emb, 1, 1)
	chunks := []chunker.Chunk{{
		ID:      "long",
		Content: long,
		Metadata: chunker.ChunkMeta{
			Path:     "long.go",
			FileType: scanner.FileTypeCode,
		},
	}}

	_, err := pool.EmbedChunks(context.Background(), chunks, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case got := <-captured:
		if len(got) != 2000 {
			t.Errorf("truncated text length = %d, want 2000", len(got))
		}
	case <-time.After(time.Second):
		t.Fatal("embedder never received a call")
	}
}

func TestPool_EmbedChunks_ContextCancelled(t *testing.T) {
	emb := &slowEmbedder{dim: 4, delay: 5 * time.Second}
	pool := NewPool(emb, 1, 1)

	chunks := make([]chunker.Chunk, 20)
	for i := range chunks {
		chunks[i] = chunker.Chunk{
			ID:      "c",
			Content: "blob",
			Metadata: chunker.ChunkMeta{
				Path:     "x.go",
				FileType: scanner.FileTypeCode,
			},
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := pool.EmbedChunks(ctx, chunks, nil)
	if err == nil {
		t.Fatal("expected context-cancelled error, got nil")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

// captureEmbedder records the first text passed to Embed so a test can verify
// truncation behaviour without inspecting the chunk slice directly.
type captureEmbedder struct {
	dim      int
	captured chan string
}

func (c *captureEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	if len(texts) > 0 {
		select {
		case c.captured <- texts[0]:
		default:
		}
	}
	results := make([][]float32, len(texts))
	for i := range results {
		results[i] = make([]float32, c.dim)
	}
	return results, nil
}

func (c *captureEmbedder) Dimensions() int { return c.dim }
