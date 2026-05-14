package cmd

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/embedder"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch [path]",
	Short: "Watch filesystem and re-index on change",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runWatch,
}

func init() {
	rootCmd.AddCommand(watchCmd)
}

func runWatch(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

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

	sc := scanner.New(cfg)
	disp := chunker.NewDispatcher(cfg.ChunkSize)
	pool := embedder.NewPool(emb, cfg.EmbedWorkers, cfg.EmbedBatchSize)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(absPath); err != nil {
		return fmt.Errorf("watching %s: %w", absPath, err)
	}

	fmt.Printf("Watching %s for changes...\n", absPath)

	// Debounce map
	var mu sync.Mutex
	pending := make(map[string]*time.Timer)

	ignorer := scanner.NewIgnorer(cfg.Ignore)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			relPath, _ := filepath.Rel(absPath, event.Name)
			if ignorer.ShouldIgnore(relPath, false) {
				continue
			}

			mu.Lock()
			if t, exists := pending[event.Name]; exists {
				t.Stop()
			}

			switch {
			case event.Has(fsnotify.Write) || event.Has(fsnotify.Create):
				pending[event.Name] = time.AfterFunc(500*time.Millisecond, func() {
					mu.Lock()
					delete(pending, event.Name)
					mu.Unlock()

					doc, err := sc.ScanFile(event.Name)
					if err != nil {
						log.Printf("Error scanning %s: %v", event.Name, err)
						return
					}

					chunks, err := disp.Chunk(*doc)
					if err != nil {
						log.Printf("Error chunking %s: %v", event.Name, err)
						return
					}

					chunks, err = pool.EmbedChunks(ctx, chunks, nil)
					if err != nil {
						log.Printf("Error embedding %s: %v", event.Name, err)
						return
					}

					if err := db.UpsertDocument(ctx, *doc, chunks); err != nil {
						log.Printf("Error storing %s: %v", event.Name, err)
						return
					}

					fmt.Printf("  Updated: %s (%d chunks)\n", relPath, len(chunks))
				})

			case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
				pending[event.Name] = time.AfterFunc(500*time.Millisecond, func() {
					mu.Lock()
					delete(pending, event.Name)
					mu.Unlock()

					docID := hashPath(relPath)
					if err := db.DeleteDocument(ctx, docID); err != nil {
						log.Printf("Error deleting %s: %v", event.Name, err)
						return
					}
					fmt.Printf("  Removed: %s\n", relPath)
				})
			}
			mu.Unlock()

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}
