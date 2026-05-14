package brain

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/store"
	"github.com/fsnotify/fsnotify"
)

// Watcher monitors ~/.claude/projects/ for conversation JSONL files
// and registers each session as a brain in the store.
type Watcher struct {
	store     store.Store
	indexer   *Indexer
	watchPath string

	mu      sync.Mutex
	pending map[string]*time.Timer
}

// NewWatcher creates a brain watcher for the given path.
// If indexer is non-nil, conversations will also be indexed into the vector store.
func NewWatcher(s store.Store, indexer *Indexer, watchPath string) *Watcher {
	return &Watcher{
		store:     s,
		indexer:   indexer,
		watchPath: watchPath,
		pending:   make(map[string]*time.Timer),
	}
}

// Start performs an initial scan then watches for changes.
// It blocks until the context is cancelled.
func (w *Watcher) Start(ctx context.Context) error {
	fmt.Printf("Scanning existing brains in %s...\n", w.watchPath)

	count, err := w.initialScan(ctx)
	if err != nil {
		return fmt.Errorf("initial scan: %w", err)
	}
	fmt.Printf("Found %d existing brain(s)\n", count)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}
	defer watcher.Close()

	// Watch the root projects directory and each subdirectory
	if err := w.addWatchRecursive(watcher, w.watchPath); err != nil {
		return fmt.Errorf("watching %s: %w", w.watchPath, err)
	}

	fmt.Printf("Watching %s for new conversations...\n", w.watchPath)

	// Periodic staleness check
	staleTicker := time.NewTicker(5 * time.Minute)
	defer staleTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			w.handleEvent(ctx, watcher, event)

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("Watcher error: %v", err)

		case <-staleTicker.C:
			w.markStaleBrains(ctx)
		}
	}
}

func (w *Watcher) initialScan(ctx context.Context) (int, error) {
	count := 0
	entries, err := os.ReadDir(w.watchPath)
	if err != nil {
		return 0, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectDir := filepath.Join(w.watchPath, entry.Name())
		files, err := os.ReadDir(projectDir)
		if err != nil {
			continue
		}

		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}

			path := filepath.Join(projectDir, f.Name())
			if err := w.processFile(ctx, path, 0, false); err != nil {
				log.Printf("Error processing %s: %v", path, err)
				continue
			}
			count++
		}
	}
	return count, nil
}

func (w *Watcher) addWatchRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
		}
		if d.IsDir() {
			if err := watcher.Add(path); err != nil {
				log.Printf("Warning: cannot watch %s: %v", path, err)
			}
		}
		return nil
	})
}

func (w *Watcher) handleEvent(ctx context.Context, watcher *fsnotify.Watcher, event fsnotify.Event) {
	// If a new directory was created, start watching it
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			watcher.Add(event.Name)
			return
		}
	}

	// Only process .jsonl files
	if !strings.HasSuffix(event.Name, ".jsonl") {
		return
	}

	// Skip subagent files for now (MVP: main conversations only)
	if strings.Contains(event.Name, "/subagents/") {
		return
	}

	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
		return
	}

	w.mu.Lock()
	if t, exists := w.pending[event.Name]; exists {
		t.Stop()
	}
	w.pending[event.Name] = time.AfterFunc(500*time.Millisecond, func() {
		w.mu.Lock()
		delete(w.pending, event.Name)
		w.mu.Unlock()

		// Look up existing brain to get offset for incremental read
		sessionID := strings.TrimSuffix(filepath.Base(event.Name), ".jsonl")
		existing, _ := w.store.GetBrain(ctx, sessionID)
		var offset int64
		if existing != nil {
			offset = existing.LastOffset
		}

		if err := w.processFile(ctx, event.Name, offset, true); err != nil {
			log.Printf("Error processing %s: %v", event.Name, err)
		}
	})
	w.mu.Unlock()
}

func (w *Watcher) processFile(ctx context.Context, path string, offset int64, doIndex bool) error {
	parsed, activity, _, err := ParseSessionFile(path, offset)
	if err != nil {
		return err
	}

	if parsed.MessageCount == 0 && offset == 0 {
		return nil // empty or metadata-only file
	}

	if offset > 0 {
		// Incremental: merge with existing
		existing, err := w.store.GetBrain(ctx, parsed.BrainID)
		if err != nil {
			return err
		}
		if existing != nil {
			MergeIncremental(existing, parsed)
			parsed = existing
		}
	}

	if err := w.store.UpsertBrain(ctx, *parsed); err != nil {
		return err
	}

	// Full re-read: clear stale activity before replacing
	if offset == 0 && len(activity) > 0 {
		w.store.DeleteBrainActivity(ctx, parsed.BrainID)
	}

	// Persist hourly activity buckets
	for _, a := range activity {
		if err := w.store.UpsertActivity(ctx, *a); err != nil {
			log.Printf("Warning: upserting activity for %s/%s: %v", a.BrainID[:8], a.Hour, err)
		}
	}

	projectName := filepath.Base(parsed.ProjectPath)
	if projectName == "" || projectName == "." {
		projectName = parsed.ProjectKey
	}

	// Index conversation content into vector store if indexer available and requested
	indexed := ""
	if doIndex && w.indexer != nil {
		if err := w.indexer.IndexBrain(ctx, parsed); err != nil {
			log.Printf("Warning: indexing %s: %v", parsed.BrainID[:8], err)
		} else {
			indexed = " [indexed]"
		}
	}

	fmt.Printf("  Brain: %s [%s] (%d msgs, %s)%s\n",
		projectName, parsed.BrainID[:8], parsed.MessageCount, parsed.Model, indexed)

	return nil
}

func (w *Watcher) markStaleBrains(ctx context.Context) {
	brains, err := w.store.ListBrains(ctx, "", StatusActive, 0)
	if err != nil {
		log.Printf("Error listing active brains: %v", err)
		return
	}

	cutoff := time.Now().Add(-30 * time.Minute)
	for _, b := range brains {
		if b.LastMessageAt == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, b.LastMessageAt)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05.000Z", b.LastMessageAt)
			if err != nil {
				continue
			}
		}
		if t.Before(cutoff) {
			b.Status = StatusCompleted
			w.store.UpsertBrain(ctx, b)
		}
	}
}
