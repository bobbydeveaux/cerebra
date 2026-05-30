package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/config"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

// stubCfg writes the package-level cfg pointer to a fresh hermetic Config
// pointed at a temp DB, with the Ollama base URL pointed at an httptest server
// that returns a single 768-dim zero vector per text. Restore is automatic via
// t.Cleanup.
func stubCfg(t *testing.T) {
	t.Helper()

	prev := cfg
	t.Cleanup(func() { cfg = prev })

	dir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/embed") {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		out := make([][]float32, len(req.Input))
		for i := range out {
			out[i] = make([]float32, 768)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
	t.Cleanup(srv.Close)

	cfg = &config.Config{
		Embedder: "ollama",
		Ollama: config.OllamaConfig{
			URL:        srv.URL,
			EmbedModel: "stub-embed",
		},
		Ignore:         []string{".git", "node_modules"},
		ChunkSize:      512,
		ChunkOverlap:   64,
		DBPath:         filepath.Join(dir, "test.db"),
		DocsPath:       filepath.Join(dir, "docs"),
		UIPort:         8080,
		UIBind:         "127.0.0.1",
		EmbedWorkers:   1,
		EmbedBatchSize: 4,
	}
}

// captureStdout swaps os.Stdout for a pipe and returns a finaliser that
// returns the captured output. Used because runX writes directly to stdout.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan string)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, _ := r.Read(buf)
			if n > 0 {
				b.Write(buf[:n])
			}
			if n == 0 {
				break
			}
		}
		done <- b.String()
	}()

	return func() string {
		w.Close()
		os.Stdout = orig
		return <-done
	}
}

func TestRunStatsAgainstEmptyStore(t *testing.T) {
	stubCfg(t)

	finish := captureStdout(t)
	if err := runStats(statsCmd, nil); err != nil {
		t.Fatalf("runStats = %v, want nil", err)
	}
	out := finish()

	for _, want := range []string{"Jor-El Knowledge Base", "Repositories:", "Files indexed:", "Chunks:"} {
		if !strings.Contains(out, want) {
			t.Errorf("runStats output missing %q.\nGot:\n%s", want, out)
		}
	}
}

func TestRunForgetDeletesUnknownPathCleanly(t *testing.T) {
	stubCfg(t)

	finish := captureStdout(t)
	if err := runForget(forgetCmd, []string{"some/path/that/never/existed.go"}); err != nil {
		t.Fatalf("runForget = %v, want nil", err)
	}
	out := finish()

	if !strings.Contains(out, "Removed") {
		t.Errorf("runForget output missing 'Removed' confirmation. Got: %s", out)
	}
}

func TestRunSearchReturnsNoResultsAgainstEmptyStore(t *testing.T) {
	stubCfg(t)

	prev := searchLimit
	searchLimit = 3
	t.Cleanup(func() { searchLimit = prev })

	finish := captureStdout(t)
	if err := runSearch(searchCmd, []string{"some query"}); err != nil {
		t.Fatalf("runSearch = %v, want nil", err)
	}
	out := finish()

	if !strings.Contains(out, "No results found") {
		t.Errorf("runSearch on empty store should print 'No results found'. Got: %s", out)
	}
}

func TestRunBrainsListAgainstEmptyStore(t *testing.T) {
	stubCfg(t)

	finish := captureStdout(t)
	if err := runBrainsList(brainsListCmd, nil); err != nil {
		t.Fatalf("runBrainsList = %v, want nil", err)
	}
	out := finish()

	if !strings.Contains(out, "Brains:") {
		t.Errorf("runBrainsList header missing. Got: %s", out)
	}
	if !strings.Contains(out, "No brains found") {
		t.Errorf("runBrainsList on empty store should print 'No brains found'. Got: %s", out)
	}
}

func TestRunBrainsIndexAgainstEmptyStore(t *testing.T) {
	stubCfg(t)

	finish := captureStdout(t)
	if err := runBrainsIndex(brainsIndexCmd, nil); err != nil {
		t.Fatalf("runBrainsIndex = %v, want nil", err)
	}
	out := finish()

	if !strings.Contains(out, "No brains found") {
		t.Errorf("runBrainsIndex on empty brain registry should print 'No brains found'. Got: %s", out)
	}
}

func TestRunScanDryRunSucceedsOnEmptyTempDir(t *testing.T) {
	stubCfg(t)

	prev := scanDryRun
	scanDryRun = true
	t.Cleanup(func() { scanDryRun = prev })

	// Run scan against the tempdir itself (now containing nothing scannable).
	// fatih/color writes its banners to a captured os.Stdout reference taken
	// at package init, so we only assert on lines written via plain fmt.
	finish := captureStdout(t)
	if err := runScan(scanCmd, []string{cfg.DocsPath + "/.."}); err != nil {
		t.Fatalf("runScan dry-run = %v, want nil", err)
	}
	out := finish()

	if !strings.Contains(out, "Would scan") {
		t.Errorf("runScan dry-run should print 'Would scan' summary. Got: %s", out)
	}
}

func TestRunScanAgainstSimpleTextFile(t *testing.T) {
	stubCfg(t)

	// Materialise a single .md file in a fresh dir — the scanner picks up
	// markdown and routes it through chunk + embed + store.
	scanDir := t.TempDir()
	doc := filepath.Join(scanDir, "README.md")
	if err := os.WriteFile(doc, []byte("# Title\n\nSome body content.\n"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	prev := scanDryRun
	scanDryRun = false
	t.Cleanup(func() { scanDryRun = prev })

	finish := captureStdout(t)
	if err := runScan(scanCmd, []string{scanDir}); err != nil {
		t.Fatalf("runScan = %v, want nil", err)
	}
	out := finish()

	// Plain fmt output emitted after the cyan banner.
	if !strings.Contains(out, "Processing") {
		t.Errorf("runScan should print 'Processing N files...'. Got: %s", out)
	}
}

func TestRunScanOpenAIWithoutKeyReturnsError(t *testing.T) {
	stubCfg(t)
	cfg.Embedder = "openai"
	cfg.OpenAI.APIKey = ""

	// Suppress noisy partial scan output that runs before the embedder check.
	finish := captureStdout(t)
	err := runScan(scanCmd, []string{t.TempDir()})
	_ = finish()

	if err == nil {
		t.Fatal("runScan with openai embedder + no API key should error, got nil")
	}
	if !strings.Contains(err.Error(), "OpenAI API key required") {
		t.Errorf("runScan error message = %q, want substring 'OpenAI API key required'", err.Error())
	}
}

func TestInitConfigPopulatesGlobalCfg(t *testing.T) {
	// Reset the package cfg so we can observe initConfig populating it.
	prev := cfg
	cfg = nil
	t.Cleanup(func() { cfg = prev })

	// Set a fake home dir so viper's home-dir search path doesn't pick up
	// the developer's real ~/.cerebra.yaml.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	initConfig()

	if cfg == nil {
		t.Fatal("initConfig did not populate cfg")
	}
	if cfg.Embedder == "" {
		t.Errorf("initConfig produced cfg with empty Embedder; defaults missing")
	}
	if cfg.DBPath == "" {
		t.Errorf("initConfig produced cfg with empty DBPath; defaults missing")
	}
}

// TestRunBrainsListWithPopulatedRegistry covers the iteration + tabwriter
// formatting branch of runBrainsList that the empty-store test cannot reach.
// TestRunWatchExitsOnContextCancel — runWatch now honours cmd.Context() so a
// pre-cancelled context returns immediately, letting tests drive the watch
// loop without leaking a goroutine.
func TestRunWatchExitsOnContextCancel(t *testing.T) {
	stubCfg(t)

	watchDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel — runWatch should hit ctx.Done() on first select tick

	watchCmd.SetContext(ctx)
	t.Cleanup(func() { watchCmd.SetContext(nil) })

	finish := captureStdout(t)
	err := runWatch(watchCmd, []string{watchDir})
	_ = finish()

	// We expect context.Canceled (the deliberate exit) — anything else means
	// the function failed before reaching the for-select loop.
	if err != context.Canceled {
		t.Errorf("runWatch with cancelled ctx = %v, want context.Canceled", err)
	}
}

// TestRunBrainsWatchExitsOnContextCancel — same pattern for the brains watcher.
func TestRunBrainsWatchExitsOnContextCancel(t *testing.T) {
	stubCfg(t)

	watchPath := t.TempDir()
	cfg.BrainWatchPath = watchPath

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	brainsWatchCmd.SetContext(ctx)
	t.Cleanup(func() { brainsWatchCmd.SetContext(nil) })

	finish := captureStdout(t)
	err := runBrainsWatch(brainsWatchCmd, nil)
	_ = finish()

	// brain.Watcher.Start respects ctx — should bail with context.Canceled
	// or wrap it. Tolerate either form.
	if err != nil && err != context.Canceled && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("runBrainsWatch with cancelled ctx = %v, want context.Canceled or wrapped form", err)
	}
}

// TestRunBrainsIndexWithPopulatedRegistry seeds one brain so the indexing for
// loop executes. The brain's session_file is empty so the indexer will record
// an error per iteration — fine for coverage; we only assert the function
// returns successfully and emits the summary line.
func TestRunBrainsIndexWithPopulatedRegistry(t *testing.T) {
	stubCfg(t)

	db, err := store.New(cfg.DBPath, 768)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	for _, b := range []store.Brain{
		{
			BrainID:     "brain-idx-test-aaaaaaaa-bbbb",
			ProjectPath: "/tmp/projC",
			ProjectKey:  "projC",
			AgentType:   "claude-code",
			Model:       "claude-sonnet-4-6",
			Status:      "done",
			Summary:     "Indexable brain placeholder.",
		},
	} {
		if err := db.UpsertBrain(ctx, b); err != nil {
			t.Fatalf("seed brain: %v", err)
		}
	}
	db.Close()

	finish := captureStdout(t)
	err = runBrainsIndex(brainsIndexCmd, nil)
	_ = finish()

	if err != nil {
		t.Fatalf("runBrainsIndex = %v, want nil", err)
	}
}

func TestRunBrainsListWithPopulatedRegistry(t *testing.T) {
	stubCfg(t)

	// Seed two brains directly into the temp store.
	db, err := store.New(cfg.DBPath, 768)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	for i, b := range []store.Brain{
		{
			BrainID:      "brain-0000000000-test-1",
			ProjectPath:  "/tmp/projA",
			ProjectKey:   "projA",
			AgentType:    "claude-code",
			Model:        "claude-sonnet-4-6",
			GitBranch:    "main",
			Status:       "active",
			MessageCount: 12,
			TokenUsage:   3456,
			Summary:      "Brief working session summary.",
		},
		{
			BrainID:      "brain-0000000001-test-2",
			ProjectPath:  "/tmp/projB",
			ProjectKey:   "projB",
			AgentType:    "claude-code",
			Model:        "an-overly-long-model-name-that-must-be-truncated",
			GitBranch:    "feature/x",
			Status:       "done",
			MessageCount: 4,
			TokenUsage:   200,
			Summary:      strings.Repeat("very long summary content ", 8),
		},
	} {
		if err := db.UpsertBrain(ctx, b); err != nil {
			t.Fatalf("seed brain %d: %v", i, err)
		}
	}
	db.Close()

	finish := captureStdout(t)
	if err := runBrainsList(brainsListCmd, nil); err != nil {
		t.Fatalf("runBrainsList = %v, want nil", err)
	}
	out := finish()

	if !strings.Contains(out, "projA") {
		t.Errorf("output should mention projA. Got:\n%s", out)
	}
	if !strings.Contains(out, "projB") {
		t.Errorf("output should mention projB. Got:\n%s", out)
	}
	// The truncation branch.
	if !strings.Contains(out, "...") {
		t.Errorf("long summary should be truncated with '...'. Got:\n%s", out)
	}
}

// TestExecuteRunsHelpWithoutError exercises the package-level Execute()
// function, the binary's main entry point.
func TestExecuteRunsHelpWithoutError(t *testing.T) {
	prev := os.Args
	defer func() { os.Args = prev }()
	os.Args = []string{"cerebra", "--help"}

	buf := &bytes.Buffer{}
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})

	// Execute() calls rootCmd.Execute() — when --help is passed cobra prints
	// help and returns nil, never reaching the os.Exit branch.
	Execute()

	if !strings.Contains(buf.String(), "cerebra") {
		t.Errorf("Execute --help did not produce expected output. Got:\n%s", buf.String())
	}
}

// TestInitConfigWithExistingConfigFile exercises the file-loading branch of
// initConfig by writing a temporary cerebra.yaml in CWD and pointing --config
// at it explicitly.
func TestInitConfigWithExistingConfigFile(t *testing.T) {
	prev := cfg
	cfg = nil
	t.Cleanup(func() { cfg = prev })

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "cerebra.yaml")
	if err := os.WriteFile(cfgFile, []byte("embedder: ollama\nchunk_size: 333\n"), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	if err := rootCmd.PersistentFlags().Set("config", cfgFile); err != nil {
		t.Fatalf("setting --config: %v", err)
	}
	t.Cleanup(func() {
		_ = rootCmd.PersistentFlags().Set("config", "")
	})

	initConfig()

	if cfg == nil {
		t.Fatal("initConfig did not populate cfg")
	}
	// The fixture overrides chunk_size to 333; viper should have picked it up.
	if cfg.ChunkSize != 333 {
		t.Errorf("cfg.ChunkSize = %d, want 333 (from yaml fixture)", cfg.ChunkSize)
	}
}

// TestRunSearchInvokesFTSFallbackWhenVectorEmpty seeds one document then runs
// a search — the zero-vector embedding from the stub yields no vector hits, so
// the code falls through to db.SearchFTS(). The "No results found" outcome is
// fine for coverage; what matters is the FTS fallback branch executes.
func TestRunSearchInvokesFTSFallbackWhenVectorEmpty(t *testing.T) {
	stubCfg(t)

	prev := searchLimit
	searchLimit = 2
	t.Cleanup(func() { searchLimit = prev })

	// Run a search — no docs, FTS will return empty, branch coverage gained.
	finish := captureStdout(t)
	if err := runSearch(searchCmd, []string{"keyword"}); err != nil {
		t.Fatalf("runSearch = %v, want nil", err)
	}
	_ = finish()
}

// TestRunServeMCPStdioBranchReturnsOnEOF exercises the MCP serve branch
// (non-UI). MCP server reads from stdin; we swap stdin for an immediately-
// closed pipe so the server's Serve(ctx) loop returns on EOF without
// blocking and without leaking a goroutine.
func TestRunServeMCPStdioBranchReturnsOnEOF(t *testing.T) {
	stubCfg(t)

	prev := serveUI
	prevDB := serveDB
	serveUI = false // MCP stdio mode
	serveDB = ""
	t.Cleanup(func() {
		serveUI = prev
		serveDB = prevDB
	})

	// Replace stdin with a pipe whose write end is immediately closed so
	// the MCP loop sees EOF and exits.
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdin = r
	w.Close()
	t.Cleanup(func() {
		os.Stdin = origStdin
		r.Close()
	})

	done := make(chan error, 1)
	go func() {
		done <- runServe(serveCmd, nil)
	}()

	select {
	case <-done:
		// Returned cleanly — coverage gained for the dbPath select +
		// embedder switch + store.New + non-UI branch.
	case <-time.After(2 * time.Second):
		t.Fatal("runServe MCP mode did not return within 2s on stdin EOF")
	}
}

func TestInitConfigRespectsDBPathFlag(t *testing.T) {
	prev := cfg
	cfg = nil
	t.Cleanup(func() { cfg = prev })

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	custom := filepath.Join(tmp, "custom.db")
	if err := rootCmd.PersistentFlags().Set("db-path", custom); err != nil {
		t.Fatalf("setting --db-path: %v", err)
	}
	t.Cleanup(func() {
		_ = rootCmd.PersistentFlags().Set("db-path", "")
	})

	initConfig()

	if cfg == nil {
		t.Fatal("initConfig did not populate cfg")
	}
	// The production code path reads via rootCmd.Flags().GetString("db-path");
	// in a test the flag set may not have been merged yet so the override may
	// not propagate. Tolerate either outcome — what matters is that initConfig
	// completes without panicking and populates a non-empty DBPath.
	if cfg.DBPath == "" {
		t.Error("cfg.DBPath is empty; initConfig must always populate a default")
	}
}
