package embedder

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
)

type Pool struct {
	embedder  Embedder
	workers   int
	batchSize int
}

func NewPool(emb Embedder, workers, batchSize int) *Pool {
	if workers <= 0 {
		workers = runtime.NumCPU()
		if workers > 4 {
			workers = 4
		}
	}
	if batchSize <= 0 {
		batchSize = 32
	}
	return &Pool{
		embedder:  emb,
		workers:   workers,
		batchSize: batchSize,
	}
}

func (p *Pool) EmbedChunks(ctx context.Context, chunks []chunker.Chunk, progress func(n int)) ([]chunker.Chunk, error) {
	if len(chunks) == 0 {
		return chunks, nil
	}

	type job struct {
		indices []int
		texts   []string
	}

	jobs := make(chan job, p.workers*2)
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup

	result := make([]chunker.Chunk, len(chunks))
	copy(result, chunks)

	// Start workers
	for w := 0; w < p.workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				mu.Lock()
				if firstErr != nil {
					mu.Unlock()
					return
				}
				mu.Unlock()

				vecs, err := p.embedder.Embed(ctx, j.texts)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("embedding batch: %w", err)
					}
					mu.Unlock()
					return
				}

				mu.Lock()
				for k, idx := range j.indices {
					result[idx].Embedding = vecs[k]
				}
				if progress != nil {
					progress(len(j.indices))
				}
				mu.Unlock()
			}
		}()
	}

	// Send batches
	for i := 0; i < len(chunks); i += p.batchSize {
		end := i + p.batchSize
		if end > len(chunks) {
			end = len(chunks)
		}

		var indices []int
		var texts []string
		for j := i; j < end; j++ {
			indices = append(indices, j)
			text := chunks[j].Content
			// Truncate to ~2000 chars to keep embedding fast
			if len(text) > 2000 {
				text = text[:2000]
			}
			texts = append(texts, text)
		}

		select {
		case jobs <- job{indices: indices, texts: texts}:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		}
	}

	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	return result, nil
}
