// Package eval provides a deterministic, API-free retrieval evaluation
// harness for Cerebra's search pipeline. Unlike evals/run.sh (which spawns
// live claude -p sessions and grades with an LLM), this harness ingests a
// small self-contained fixture corpus and asserts that the FTS keyword path
// surfaces the expected facts in the top-N results. It needs no embeddings,
// no network, and no API keys, so it can gate every pull request in CI.
package eval

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

//go:embed fixtures/*.md
var fixturesFS embed.FS

// Question is one retrieval check: run Query through FTS search and require
// every term in MustContain to appear (case-insensitively) somewhere in the
// top-N results' content. Category is informational, used in the report.
type Question struct {
	ID          string
	Category    string
	Query       string
	MustContain []string
}

// Result is the outcome of evaluating one Question.
type Result struct {
	Question Question
	Pass     bool
	Hits     int
	Missing  string
}

// Report aggregates the run.
type Report struct {
	Results  []Result
	Pass     int
	Fail     int
	Total    int
	PassRate float64
}

// Meets reports whether the run cleared the given pass-rate threshold (0..1).
func (r Report) Meets(threshold float64) bool {
	return r.PassRate >= threshold
}

// Questions returns the default CI question set. Every expected fact is
// answerable from the embedded fixture corpus alone, so the suite is
// deterministic and self-contained.
func Questions() []Question {
	return []Question{
		{
			ID:       "C01",
			Category: "engineering",
			Query:    "difference between Cerebra and Fortress fork",
			MustContain: []string{
				"fork", "Fortress",
			},
		},
		{
			ID:       "C02",
			Category: "engineering",
			Query:    "token counting per brain double counting bug",
			MustContain: []string{
				"double", "cache_read_input_tokens",
			},
		},
		{
			ID:       "C03",
			Category: "search",
			Query:    "FTS keyword search no embeddings external API",
			MustContain: []string{
				"keyword", "chunks_fts",
			},
		},
		{
			ID:       "C04",
			Category: "mcp",
			Query:    "MCP tools list_brains search_agent activity",
			MustContain: []string{
				"list_brains", "search_agent",
			},
		},
		{
			ID:       "C05",
			Category: "build",
			Query:    "sqlite_fts5 build tag full text index",
			MustContain: []string{
				"sqlite_fts5", "chunks_fts",
			},
		},
		{
			ID:       "C06",
			Category: "lookup",
			Query:    "Cerebra Jor-El SQLite vector database",
			MustContain: []string{
				"Jor-El", "SQLite",
			},
		},
		{
			ID:       "C07",
			Category: "lookup",
			Query:    "vector search falls back to FTS no results",
			MustContain: []string{
				"falls back", "FTS",
			},
		},
	}
}

// Seed ingests the embedded fixture corpus into the store via the normal
// UpsertDocument path, one chunk per fixture file.
func Seed(ctx context.Context, db *store.SQLiteStore) error {
	entries, err := fs.ReadDir(fixturesFS, "fixtures")
	if err != nil {
		return fmt.Errorf("reading fixtures: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		rel := e.Name()
		b, err := fixturesFS.ReadFile(filepath.Join("fixtures", rel))
		if err != nil {
			return fmt.Errorf("reading fixture %s: %w", rel, err)
		}
		content := string(b)
		id := strings.TrimSuffix(rel, filepath.Ext(rel))
		doc := scanner.Document{
			ID:          "fixture-" + id,
			Path:        "/eval/fixtures/" + rel,
			RelPath:     rel,
			Category:    scanner.CategoryDocs,
			Language:    "markdown",
			FileType:    scanner.FileTypeMarkdown,
			Content:     content,
			ContentHash: id,
			Metadata:    map[string]string{},
		}
		chunks := []chunker.Chunk{
			{
				ID:         "fixture-" + id + "-c1",
				DocumentID: doc.ID,
				Content:    content,
				StartLine:  1,
				EndLine:    strings.Count(content, "\n") + 1,
				Metadata: chunker.ChunkMeta{
					Path:     rel,
					Category: scanner.CategoryDocs,
					Language: "markdown",
					FileType: scanner.FileTypeMarkdown,
				},
			},
		}
		if err := db.UpsertDocument(ctx, doc, chunks); err != nil {
			return fmt.Errorf("seeding fixture %s: %w", rel, err)
		}
	}
	return nil
}

// Run evaluates every question against the store using FTS retrieval only.
// topN bounds how many results are inspected per query; <=0 defaults to 5.
func Run(ctx context.Context, db *store.SQLiteStore, qs []Question, topN int) (Report, error) {
	if topN <= 0 {
		topN = 5
	}
	rep := Report{Total: len(qs)}
	for _, q := range qs {
		results, err := db.SearchFTS(ctx, q.Query, topN)
		if err != nil {
			return rep, fmt.Errorf("FTS search for %s: %w", q.ID, err)
		}
		var haystack strings.Builder
		for _, r := range results {
			haystack.WriteString(r.Chunk.Content)
			haystack.WriteByte('\n')
			haystack.WriteString(r.Highlight)
			haystack.WriteByte('\n')
		}
		hay := strings.ToLower(haystack.String())
		res := Result{Question: q, Hits: len(results), Pass: true}
		for _, term := range q.MustContain {
			if !strings.Contains(hay, strings.ToLower(term)) {
				res.Pass = false
				res.Missing = term
				break
			}
		}
		if res.Pass {
			rep.Pass++
		} else {
			rep.Fail++
		}
		rep.Results = append(rep.Results, res)
	}
	if rep.Total > 0 {
		rep.PassRate = float64(rep.Pass) / float64(rep.Total)
	}
	return rep, nil
}
