package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/bobbydeveaux/cerebra/internal/store"
	"github.com/spf13/cobra"
)

var forgetCmd = &cobra.Command{
	Use:   "forget [path]",
	Short: "Remove a path from the knowledge base",
	Args:  cobra.ExactArgs(1),
	RunE:  runForget,
}

func init() {
	rootCmd.AddCommand(forgetCmd)
}

func runForget(cmd *cobra.Command, args []string) error {
	path := args[0]
	ctx := context.Background()

	db, err := store.New(cfg.DBPath, 768)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	docID := fmt.Sprintf("%x", sha256.Sum256([]byte(path)))

	if err := db.DeleteDocument(ctx, docID); err != nil {
		return fmt.Errorf("deleting document: %w", err)
	}

	fmt.Printf("Removed %s from knowledge base.\n", path)
	return nil
}
