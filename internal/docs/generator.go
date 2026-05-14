package docs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/store"
)

type Generator struct {
	store    store.Store
	docsPath string
}

func NewGenerator(s store.Store, docsPath string) *Generator {
	return &Generator{store: s, docsPath: docsPath}
}

func (g *Generator) Generate(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Join(g.docsPath, "categories"), 0755); err != nil {
		return fmt.Errorf("creating docs directory: %w", err)
	}

	stats, err := g.store.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("getting stats: %w", err)
	}

	categories, err := g.store.ListCategories(ctx)
	if err != nil {
		return fmt.Errorf("listing categories: %w", err)
	}

	if err := g.generateIndex(stats, categories); err != nil {
		return fmt.Errorf("generating index: %w", err)
	}

	for _, cat := range categories {
		if err := g.generateCategory(ctx, cat); err != nil {
			return fmt.Errorf("generating category %s: %w", cat.Name, err)
		}
	}

	return nil
}

func (g *Generator) generateIndex(stats store.Stats, categories []store.CategorySummary) error {
	var sb strings.Builder

	sb.WriteString("# Jor-El Knowledge Base Index\n\n")
	sb.WriteString(fmt.Sprintf("*Generated: %s*\n\n", time.Now().Format(time.RFC3339)))

	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Metric | Value |\n")
	sb.WriteString("|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Repositories | %d |\n", stats.Repos))
	sb.WriteString(fmt.Sprintf("| Files indexed | %d |\n", stats.Files))
	sb.WriteString(fmt.Sprintf("| Chunks | %d |\n", stats.Chunks))
	sb.WriteString(fmt.Sprintf("| Categories | %d |\n", stats.Categories))
	sb.WriteString(fmt.Sprintf("| DB size | %.2f MB |\n", stats.DBSizeMB))
	if stats.LastScan != "" {
		sb.WriteString(fmt.Sprintf("| Last scan | %s |\n", stats.LastScan))
	}

	sb.WriteString("\n## Categories\n\n")
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].FileCount > categories[j].FileCount
	})

	for _, cat := range categories {
		sb.WriteString(fmt.Sprintf("- [%s](categories/%s.md) — %d files, %d chunks\n",
			cat.Name, cat.Name, cat.FileCount, cat.ChunkCount))
	}

	return os.WriteFile(filepath.Join(g.docsPath, "index.md"), []byte(sb.String()), 0644)
}

func (g *Generator) generateCategory(ctx context.Context, cat store.CategorySummary) error {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Category: %s\n\n", cat.Name))
	sb.WriteString(fmt.Sprintf("*%d files, %d chunks*\n\n", cat.FileCount, cat.ChunkCount))

	// List files in this category
	files, err := g.store.ListFilesByCategory(ctx, cat.Name, 10000)
	if err == nil && len(files) > 0 {
		// Group by directory
		dirs := make(map[string][]store.FileSummary)
		for _, f := range files {
			dir := filepath.Dir(f.RelPath)
			dirs[dir] = append(dirs[dir], f)
		}

		// Sort directory names
		dirNames := make([]string, 0, len(dirs))
		for d := range dirs {
			dirNames = append(dirNames, d)
		}
		sort.Strings(dirNames)

		sb.WriteString("## Files\n\n")
		for _, dir := range dirNames {
			dirFiles := dirs[dir]
			sb.WriteString(fmt.Sprintf("### `%s/`\n\n", dir))
			for _, f := range dirFiles {
				lang := ""
				if f.Language != "" {
					lang = fmt.Sprintf(" (%s)", f.Language)
				}
				sb.WriteString(fmt.Sprintf("- `%s`%s\n", filepath.Base(f.RelPath), lang))
			}
			sb.WriteString("\n")
		}
	}

	return os.WriteFile(
		filepath.Join(g.docsPath, "categories", cat.Name+".md"),
		[]byte(sb.String()), 0644)
}
