package cmd

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/datasource/confluence"
	"github.com/bobbydeveaux/cerebra/internal/docs"
	"github.com/bobbydeveaux/cerebra/internal/embedder"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var scanCmd = &cobra.Command{
	Use:   "scan [path]",
	Short: "Scan, chunk, embed, and store codebase knowledge",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runScan,
}

var (
	scanDryRun    bool
	scanFull      bool
	scanUpload    string
	scanConfluence bool
)

func init() {
	scanCmd.Flags().BoolVar(&scanDryRun, "dry-run", false, "Preview what would be scanned")
	scanCmd.Flags().BoolVar(&scanFull, "full", false, "Force full re-scan")
	scanCmd.Flags().StringVar(&scanUpload, "upload", "", "Upload DB to cloud storage URI after scan")
	scanCmd.Flags().BoolVar(&scanConfluence, "confluence", false, "Also scan Confluence spaces")
	scanCmd.Flags().Bool("ci", false, "CI mode")
	rootCmd.AddCommand(scanCmd)
}

func runScan(cmd *cobra.Command, args []string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	ctx := context.Background()

	// Create embedder
	var emb embedder.Embedder
	switch cfg.Embedder {
	case "openai":
		if cfg.OpenAI.APIKey == "" {
			return fmt.Errorf("OpenAI API key required (set OPENAI_API_KEY or configure in cerebra.yaml)")
		}
		emb = embedder.NewOpenAI(cfg.OpenAI.APIKey, cfg.OpenAI.EmbedModel)
	default:
		emb = embedder.NewOllama(cfg.Ollama.URL, cfg.Ollama.EmbedModel)
	}

	// Create store
	db, err := store.New(cfg.DBPath, emb.Dimensions())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	// Create scanner
	sc := scanner.New(cfg)

	cyan := color.New(color.FgCyan)
	cyan.Printf("Scanning %s...\n", path)

	docCh, errCh := sc.Scan(ctx, path)

	// Collect documents
	var documents []scanner.Document
	var scanErrors []error
	var dryRunCount int

	go func() {
		for err := range errCh {
			scanErrors = append(scanErrors, err)
		}
	}()

	for doc := range docCh {
		if scanDryRun {
			fmt.Printf("  %s [%s/%s]\n", doc.RelPath, doc.FileType, doc.Category)
			dryRunCount++
			continue
		}

		if !scanFull {
			existingHash, _ := db.GetContentHash(ctx, doc.ID)
			if existingHash == doc.ContentHash {
				continue
			}
		}

		documents = append(documents, doc)
	}

	if scanDryRun {
		fmt.Printf("\nWould scan %d files\n", dryRunCount)
		return nil
	}

	// Extract git history as documents
	gitRepos := make(map[string]string) // repo name -> repo root
	for _, doc := range documents {
		if doc.Repo != "" && doc.RepoRoot != "" {
			gitRepos[doc.Repo] = doc.RepoRoot
		}
	}
	for repoName, repoRoot := range gitRepos {
		info, err := scanner.ExtractGitInfo(repoRoot, 200)
		if err != nil || info.Log == "" {
			continue
		}
		gitDocID := hashPath("git-history:" + repoName)
		gitDoc := scanner.Document{
			ID:          gitDocID,
			Path:        repoRoot + "/.git/log",
			RelPath:     ".git-history/" + repoName,
			Repo:        repoName,
			RepoRoot:    repoRoot,
			Category:    scanner.CategoryDocs,
			Language:    "",
			FileType:    scanner.FileTypeGitHistory,
			Content:     fmt.Sprintf("[REPO: %s] Git History\n\n%s", repoName, info.Log),
			ContentHash: hashPath(info.Log),
			Metadata: map[string]string{
				"remote_url": info.RemoteURL,
				"head_sha":   info.LastCommitSHA,
			},
		}
		documents = append(documents, gitDoc)
	}

	// Scan Confluence if configured
	if scanConfluence && cfg.Confluence.BaseURL != "" {
		cyan.Println("Scanning Confluence...")
		cp := confluence.New(
			cfg.Confluence.BaseURL,
			cfg.Confluence.Email,
			cfg.Confluence.APIToken,
			cfg.Confluence.SpaceKeys,
		)
		cDocCh, cErrCh := cp.Scan(ctx)

		go func() {
			for err := range cErrCh {
				fmt.Fprintf(os.Stderr, "Confluence error: %v\n", err)
				scanErrors = append(scanErrors, err)
			}
		}()

		var confluenceCount int
		for doc := range cDocCh {
			if !scanFull {
				existingHash, _ := db.GetContentHash(ctx, doc.ID)
				if existingHash == doc.ContentHash {
					continue
				}
			}
			documents = append(documents, doc)
			confluenceCount++
		}
		fmt.Printf("Found %s Confluence pages to index\n", color.YellowString("%d", confluenceCount))
	}

	if len(documents) == 0 {
		fmt.Println("No changes detected.")
		return nil
	}

	fmt.Printf("Processing %s files...\n", color.YellowString("%d", len(documents)))

	// Chunk all documents
	dispatcher := chunker.NewDispatcher(cfg.ChunkSize)
	var allChunks []chunker.Chunk
	docChunks := make(map[string][]chunker.Chunk)

	for _, doc := range documents {
		chunks, err := dispatcher.Chunk(doc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: chunking %s: %v\n", doc.RelPath, err)
			continue
		}
		allChunks = append(allChunks, chunks...)
		docChunks[doc.ID] = chunks
	}

	fmt.Printf("Generated %s chunks, embedding...\n", color.YellowString("%d", len(allChunks)))

	// Embed all chunks
	pool := embedder.NewPool(emb, cfg.EmbedWorkers, cfg.EmbedBatchSize)

	bar := progressbar.NewOptions(len(allChunks),
		progressbar.OptionSetDescription("Embedding"),
		progressbar.OptionShowCount(),
		progressbar.OptionSetWidth(40),
	)

	embeddedChunks, err := pool.EmbedChunks(ctx, allChunks, func(n int) {
		bar.Add(n)
	})
	if err != nil {
		return fmt.Errorf("embedding: %w", err)
	}
	bar.Finish()
	fmt.Println()

	// Rebuild docChunks map with embeddings
	idx := 0
	for _, doc := range documents {
		n := len(docChunks[doc.ID])
		docChunks[doc.ID] = embeddedChunks[idx : idx+n]
		idx += n
	}

	// Store documents
	fmt.Println("Storing documents...")
	repoChunks := make(map[string]int)
	repoFiles := make(map[string]int)

	for _, doc := range documents {
		chunks := docChunks[doc.ID]
		if err := db.UpsertDocument(ctx, doc, chunks); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: storing %s: %v\n", doc.RelPath, err)
			continue
		}
		if doc.Repo != "" {
			repoFiles[doc.Repo]++
			repoChunks[doc.Repo] += len(chunks)
		}
	}

	// Update scan state per repo
	for repo, fileCount := range repoFiles {
		state := store.ScanState{
			Repo:         repo,
			LastScanTime: time.Now(),
			FileCount:    fileCount,
			ChunkCount:   repoChunks[repo],
		}
		// Try to get the latest commit SHA
		for _, doc := range documents {
			if doc.Repo == repo && doc.RepoRoot != "" {
				info, err := scanner.ExtractGitInfo(doc.RepoRoot, 0)
				if err == nil {
					state.LastCommitSHA = info.LastCommitSHA
				}
				break
			}
		}
		db.UpdateScanState(ctx, state)
	}

	// Generate docs
	fmt.Println("Generating documentation...")
	gen := docs.NewGenerator(db, cfg.DocsPath)
	if err := gen.Generate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: generating docs: %v\n", err)
	}

	// Upload if requested
	if scanUpload != "" {
		fmt.Printf("Uploading to %s...\n", scanUpload)
		// Cloud storage upload handled separately
	}

	stats, _ := db.GetStats(ctx)

	green := color.New(color.FgGreen, color.Bold)
	yellow := color.New(color.FgYellow)
	dim := color.New(color.FgHiBlack)

	fmt.Println()
	green.Println("Done! Jor-El now knows:")
	fmt.Println()
	dim.Print("  Repos:      ")
	yellow.Printf("%d\n", stats.Repos)
	dim.Print("  Files:      ")
	yellow.Printf("%d\n", stats.Files)
	dim.Print("  Chunks:     ")
	yellow.Printf("%d\n", stats.Chunks)
	dim.Print("  Categories: ")
	yellow.Printf("%d\n", stats.Categories)
	dim.Print("  DB size:    ")
	cyan.Printf("%.2f MB\n", stats.DBSizeMB)

	if len(scanErrors) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d scan errors (non-fatal)\n", len(scanErrors))
	}

	return nil
}

func hashPath(path string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(path)))
}
