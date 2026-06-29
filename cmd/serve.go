package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/bobbydeveaux/cerebra/internal/embedder"
	"github.com/bobbydeveaux/cerebra/internal/mcp"
	"github.com/bobbydeveaux/cerebra/internal/rag"
	"github.com/bobbydeveaux/cerebra/internal/store"
	"github.com/bobbydeveaux/cerebra/internal/web"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start MCP server (default) or web UI",
	RunE:  runServe,
}

var (
	serveUI      bool
	servePort    int
	serveDB      string
	serveHTTP    bool
	serveAddr    string
	serveDataDir string
	serveSSEPath string
)

func init() {
	serveCmd.Flags().BoolVar(&serveUI, "ui", false, "Start web UI instead of MCP server")
	serveCmd.Flags().IntVar(&servePort, "port", 0, "Web UI port (default from config)")
	serveCmd.Flags().StringVar(&serveDB, "db", "", "Database URI (local path or cloud URI)")
	serveCmd.Flags().BoolVar(&serveHTTP, "http", false, "Start the multi-tenant HTTP MCP transport (SSE) instead of stdio")
	serveCmd.Flags().StringVar(&serveAddr, "addr", ":7070", "Listen address for the HTTP MCP transport")
	serveCmd.Flags().StringVar(&serveDataDir, "data-dir", "", "Base directory holding per-user databases (required with --http)")
	serveCmd.Flags().StringVar(&serveSSEPath, "sse-path", "/mcp/sse", "SSE endpoint path for the HTTP MCP transport")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	dbPath := cfg.DBPath
	if serveDB != "" {
		dbPath = serveDB
	}

	var emb embedder.Embedder
	switch cfg.Embedder {
	case "openai":
		emb = embedder.NewOpenAI(cfg.OpenAI.APIKey, cfg.OpenAI.EmbedModel)
	default:
		emb = embedder.NewOllama(cfg.Ollama.URL, cfg.Ollama.EmbedModel)
	}

	// Multi-tenant HTTP MCP transport. This mode does NOT open the single
	// shared store — each user gets their own database under --data-dir, opened
	// on demand by the registry.
	if serveHTTP {
		return runServeHTTP(emb)
	}

	db, err := store.New(dbPath, emb.Dimensions())
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	if serveUI {
		port := cfg.UIPort
		if servePort > 0 {
			port = servePort
		}

		pipeline := rag.NewPipeline(emb, db, cfg)
		srv := web.NewServer(db, emb, pipeline, cfg)

		addr := fmt.Sprintf("%s:%d", cfg.UIBind, port)
		fmt.Printf("Cerebra web UI: http://%s\n", addr)

		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("listening: %w", err)
		}
		return srv.Serve(ln)
	}

	// MCP server mode (stdio)
	server := mcp.NewServer(db, emb)
	return server.Serve(ctx)
}

// runServeHTTP starts the multi-tenant HTTP MCP (SSE) transport. It fails
// closed: without CEREBRA_TOKEN_SECRET it refuses to start, so we never run an
// unauthenticated transport. Each authenticated request is routed to that
// user's own database under --data-dir.
func runServeHTTP(emb embedder.Embedder) error {
	secret := os.Getenv("CEREBRA_TOKEN_SECRET")
	if secret == "" {
		return fmt.Errorf("CEREBRA_TOKEN_SECRET is not set; refusing to start the HTTP MCP transport unauthenticated")
	}

	dataDir := serveDataDir
	if dataDir == "" {
		return fmt.Errorf("--data-dir is required with --http (base directory for per-user databases)")
	}

	registry := mcp.NewRegistry(dataDir, emb)
	defer registry.Close()

	httpSrv, err := mcp.NewHTTPServer(registry, secret)
	if err != nil {
		return fmt.Errorf("creating HTTP MCP transport: %w", err)
	}
	if serveSSEPath != "" {
		httpSrv.SetSSEPath(serveSSEPath)
	}

	ln, err := net.Listen("tcp", serveAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", serveAddr, err)
	}

	fmt.Printf("Cerebra multi-tenant MCP (SSE) on http://%s%s (data-dir: %s)\n", serveAddr, serveSSEPath, dataDir)
	return http.Serve(ln, httpSrv.Handler())
}
