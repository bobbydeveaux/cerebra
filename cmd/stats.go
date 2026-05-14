package cmd

import (
	"context"
	"fmt"

	"github.com/bobbydeveaux/cerebra/internal/store"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show knowledge base summary",
	RunE:  runStats,
}

func init() {
	rootCmd.AddCommand(statsCmd)
}

func runStats(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	db, err := store.New(cfg.DBPath, 768)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	stats, err := db.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("getting stats: %w", err)
	}

	fmt.Println("Jor-El Knowledge Base")
	fmt.Println("=====================")
	fmt.Printf("  Repositories:  %d\n", stats.Repos)
	fmt.Printf("  Files indexed: %d\n", stats.Files)
	fmt.Printf("  Chunks:        %d\n", stats.Chunks)
	fmt.Printf("  Categories:    %d\n", stats.Categories)
	fmt.Printf("  DB size:       %.2f MB\n", stats.DBSizeMB)
	if stats.LastScan != "" {
		fmt.Printf("  Last scan:     %s\n", stats.LastScan)
	}

	cats, err := db.ListCategories(ctx)
	if err == nil && len(cats) > 0 {
		fmt.Println("\nCategories:")
		for _, c := range cats {
			fmt.Printf("  %-20s %d files, %d chunks\n", c.Name, c.FileCount, c.ChunkCount)
		}
	}

	return nil
}
