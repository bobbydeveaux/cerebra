package cmd

import (
	"context"
	"fmt"

	"github.com/bobbydeveaux/cerebra/internal/embedder"
	"github.com/bobbydeveaux/cerebra/internal/store"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Semantic search across the knowledge base",
	Args:  cobra.ExactArgs(1),
	RunE:  runSearch,
}

var searchLimit int

func init() {
	searchCmd.Flags().IntVar(&searchLimit, "limit", 5, "Number of results")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	ctx := context.Background()

	var emb embedder.Embedder
	switch cfg.Embedder {
	case "openai":
		emb = embedder.NewOpenAI(cfg.OpenAI.APIKey, cfg.OpenAI.EmbedModel)
	default:
		emb = embedder.NewOllama(cfg.Ollama.URL, cfg.Ollama.EmbedModel)
	}

	db, err := store.New(cfg.DBPath, emb.Dimensions())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Embed query
	vecs, err := emb.Embed(ctx, []string{query})
	if err != nil {
		return fmt.Errorf("embedding query: %w", err)
	}

	// Vector search
	results, err := db.Search(ctx, vecs[0], searchLimit)
	if err != nil {
		return fmt.Errorf("searching: %w", err)
	}

	if len(results) == 0 {
		// Fallback to FTS
		results, err = db.SearchFTS(ctx, query, searchLimit)
		if err != nil {
			return fmt.Errorf("FTS search: %w", err)
		}
	}

	if len(results) == 0 {
		fmt.Println("No results found.")
		return nil
	}

	for i, r := range results {
		fmt.Printf("\n--- Result %d (score: %.4f) ---\n", i+1, r.Score)
		fmt.Printf("File: %s", r.Chunk.Metadata.Path)
		if r.Chunk.StartLine > 0 {
			fmt.Printf(":%d-%d", r.Chunk.StartLine, r.Chunk.EndLine)
		}
		fmt.Println()
		if r.Chunk.Metadata.Repo != "" {
			fmt.Printf("Repo: %s  Category: %s\n", r.Chunk.Metadata.Repo, r.Chunk.Metadata.Category)
		}
		fmt.Println()

		content := r.Chunk.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		fmt.Println(content)
	}

	return nil
}
