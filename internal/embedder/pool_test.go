package embedder

import (
	"context"
	"testing"

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
