package store

import (
	"context"
	"fmt"
	"strings"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
)

func (s *SQLiteStore) Search(ctx context.Context, queryVec []float32, limit int) ([]SearchResult, error) {
	if !s.vecAvailable {
		return nil, fmt.Errorf("vector search unavailable: sqlite-vec extension not loaded")
	}

	if limit <= 0 {
		limit = 5
	}

	vecBytes, err := sqlite_vec.SerializeFloat32(queryVec)
	if err != nil {
		return nil, fmt.Errorf("serializing query vector: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT ce.chunk_id, ce.distance,
		        c.content, c.document_id, c.start_line, c.end_line,
		        d.rel_path, d.repo_id, d.category, d.language, d.file_type
		 FROM chunk_embeddings ce
		 JOIN chunks c ON c.id = ce.chunk_id
		 JOIN documents d ON d.id = c.document_id
		 WHERE ce.embedding MATCH ?
		   AND k = ?
		 ORDER BY ce.distance`,
		vecBytes, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var (
			chunkID  string
			distance float32
			content  string
			docID    string
			start    int
			end      int
			path     string
			repo     *string
			cat      string
			lang     string
			ft       string
		)
		if err := rows.Scan(&chunkID, &distance, &content, &docID, &start, &end,
			&path, &repo, &cat, &lang, &ft); err != nil {
			continue
		}

		repoStr := ""
		if repo != nil {
			repoStr = *repo
		}

		results = append(results, SearchResult{
			Chunk: chunker.Chunk{
				ID:         chunkID,
				DocumentID: docID,
				Content:    content,
				StartLine:  start,
				EndLine:    end,
				Metadata: chunker.ChunkMeta{
					Path:     path,
					Repo:     repoStr,
					Category: scanner.Category(cat),
					Language: lang,
					FileType: scanner.FileType(ft),
				},
			},
			Score: 1 - distance, // Convert distance to similarity
		})
	}

	return results, nil
}

func (s *SQLiteStore) SearchFTS(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}

	// Escape FTS special characters
	query = strings.ReplaceAll(query, `"`, `""`)

	rows, err := s.db.QueryContext(ctx,
		`SELECT f.chunk_id, f.rank, f.content, f.document_id,
		        c.start_line, c.end_line,
		        d.rel_path, d.repo_id, d.category, d.language, d.file_type,
		        snippet(chunks_fts, 0, '<b>', '</b>', '...', 32) as highlight
		 FROM chunks_fts f
		 JOIN chunks c ON c.id = f.chunk_id
		 JOIN documents d ON d.id = c.document_id
		 WHERE chunks_fts MATCH ?
		 ORDER BY f.rank
		 LIMIT ?`,
		query, limit)
	if err != nil {
		return nil, fmt.Errorf("FTS search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var (
			chunkID   string
			rank      float32
			content   string
			docID     string
			start     int
			end       int
			path      string
			repo      *string
			cat       string
			lang      string
			ft        string
			highlight string
		)
		if err := rows.Scan(&chunkID, &rank, &content, &docID, &start, &end,
			&path, &repo, &cat, &lang, &ft, &highlight); err != nil {
			continue
		}

		repoStr := ""
		if repo != nil {
			repoStr = *repo
		}

		results = append(results, SearchResult{
			Chunk: chunker.Chunk{
				ID:         chunkID,
				DocumentID: docID,
				Content:    content,
				StartLine:  start,
				EndLine:    end,
				Metadata: chunker.ChunkMeta{
					Path:     path,
					Repo:     repoStr,
					Category: scanner.Category(cat),
					Language: lang,
					FileType: scanner.FileType(ft),
				},
			},
			Score:     -rank,
			Highlight: highlight,
		})
	}

	return results, nil
}
