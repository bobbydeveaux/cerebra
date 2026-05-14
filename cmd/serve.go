package cmd

import (
	"context"
	"fmt"
	"net"

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
	serveUI   bool
	servePort int
	serveDB   string
)

func init() {
	serveCmd.Flags().BoolVar(&serveUI, "ui", false, "Start web UI instead of MCP server")
	serveCmd.Flags().IntVar(&servePort, "port", 0, "Web UI port (default from config)")
	serveCmd.Flags().StringVar(&serveDB, "db", "", "Database URI (local path or cloud URI)")
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
