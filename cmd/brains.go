package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/bobbydeveaux/cerebra/internal/brain"
	"github.com/bobbydeveaux/cerebra/internal/embedder"
	"github.com/bobbydeveaux/cerebra/internal/store"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var brainsCmd = &cobra.Command{
	Use:   "brains",
	Short: "Manage agent brain registry",
	Long:  "Discover, watch, and list AI agent conversation brains.",
}

var brainsWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch for Claude Code conversations, register and index brains",
	RunE:  runBrainsWatch,
}

var brainsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all known agent brains",
	RunE:  runBrainsList,
}

var brainsIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Index all known brain conversations into the vector store",
	RunE:  runBrainsIndex,
}

func init() {
	rootCmd.AddCommand(brainsCmd)
	brainsCmd.AddCommand(brainsWatchCmd)
	brainsCmd.AddCommand(brainsListCmd)
	brainsCmd.AddCommand(brainsIndexCmd)
}

func createEmbedder() embedder.Embedder {
	switch cfg.Embedder {
	case "openai":
		return embedder.NewOpenAI(cfg.OpenAI.APIKey, cfg.OpenAI.EmbedModel)
	default:
		return embedder.NewOllama(cfg.Ollama.URL, cfg.Ollama.EmbedModel)
	}
}

func runBrainsWatch(cmd *cobra.Command, args []string) error {
	watchPath := cfg.BrainWatchPath
	if watchPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		watchPath = filepath.Join(home, ".claude", "projects")
	}

	emb := createEmbedder()
	db, err := store.New(cfg.DBPath, emb.Dimensions())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	indexer := brain.NewIndexer(db, emb, cfg.EmbedWorkers, cfg.EmbedBatchSize, cfg.ChunkSize)
	w := brain.NewWatcher(db, indexer, watchPath)
	// Honour the cobra command context so callers can cancel cleanly.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	return w.Start(ctx)
}

func runBrainsIndex(cmd *cobra.Command, args []string) error {
	emb := createEmbedder()
	db, err := store.New(cfg.DBPath, emb.Dimensions())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	ctx := context.Background()

	brains, err := db.ListBrains(ctx, "", "", 0)
	if err != nil {
		return err
	}

	if len(brains) == 0 {
		fmt.Println("No brains found. Run 'cerebra brains watch' first to discover conversations.")
		return nil
	}

	fmt.Printf("Indexing %d brain conversations...\n", len(brains))

	indexer := brain.NewIndexer(db, emb, cfg.EmbedWorkers, cfg.EmbedBatchSize, cfg.ChunkSize)

	bar := progressbar.NewOptions(len(brains),
		progressbar.OptionSetDescription("Indexing"),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
	)

	indexed := 0
	skipped := 0
	errors := 0

	for i := range brains {
		if err := indexer.IndexBrain(ctx, &brains[i]); err != nil {
			fmt.Fprintf(os.Stderr, "\nWarning: %s: %v\n", brains[i].BrainID[:8], err)
			errors++
		} else {
			indexed++
		}
		bar.Add(1)
	}
	bar.Finish()
	fmt.Println()

	fmt.Printf("Done! Indexed: %d, Skipped (unchanged): %d, Errors: %d\n",
		indexed, skipped, errors)

	stats, _ := db.GetStats(ctx)
	fmt.Printf("DB now has %d files, %d chunks (%.2f MB)\n",
		stats.Files, stats.Chunks, stats.DBSizeMB)

	return nil
}

func runBrainsList(cmd *cobra.Command, args []string) error {
	db, err := store.New(cfg.DBPath, cfg.EmbedDimensions())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	ctx := context.Background()

	stats, err := db.GetBrainStats(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Brains: %d (%d active) | Projects: %d | Messages: %d | Tokens: %d\n\n",
		stats.TotalBrains, stats.ActiveBrains, stats.Projects, stats.TotalMessages, stats.TotalTokens)

	brains, err := db.ListBrains(ctx, "", "", 50)
	if err != nil {
		return err
	}

	if len(brains) == 0 {
		fmt.Println("No brains found. Run 'cerebra brains watch' to discover conversations.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STATUS\tPROJECT\tMODEL\tMSGS\tTOKENS\tBRANCH\tSUMMARY")
	fmt.Fprintln(tw, "------\t-------\t-----\t----\t------\t------\t-------")
	for _, b := range brains {
		project := filepath.Base(b.ProjectPath)
		if project == "" || project == "." {
			project = b.ProjectKey
		}
		summary := b.Summary
		if len(summary) > 60 {
			summary = summary[:57] + "..."
		}
		model := b.Model
		if len(model) > 20 {
			model = model[:20]
		}
		status := "done"
		if b.Status == brain.StatusActive {
			status = "LIVE"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%s\t%s\n",
			status, project, model, b.MessageCount, b.TokenUsage, b.GitBranch, summary)
	}
	tw.Flush()

	return nil
}
