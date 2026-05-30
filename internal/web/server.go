package web

import (
	"embed"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/bobbydeveaux/cerebra/internal/config"
	"github.com/bobbydeveaux/cerebra/internal/embedder"
	"github.com/bobbydeveaux/cerebra/internal/rag"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

type Server struct {
	store         store.Store
	embedder      embedder.Embedder
	pipeline      *rag.Pipeline
	cfg           *config.Config
	tmpls         map[string]*template.Template
	mux           *http.ServeMux
	stripeHandler StripeEventHandler
	logger        *slog.Logger
}

func NewServer(s store.Store, emb embedder.Embedder, p *rag.Pipeline, cfg *config.Config) *Server {
	srv := &Server{
		store:         s,
		embedder:      emb,
		pipeline:      p,
		cfg:           cfg,
		mux:           http.NewServeMux(),
		tmpls:         make(map[string]*template.Template),
		stripeHandler: loggingStripeHandler{},
		logger:        slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	}

	funcMap := template.FuncMap{
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"formatTokens": func(n int) string {
			return formatTokenCount(n)
		},
	}

	// Parse each page template separately with the layout to avoid
	// conflicting "content" block definitions overwriting each other.
	pages := []string{"index.html", "category.html", "document.html", "search.html", "chat.html", "brains.html"}
	for _, page := range pages {
		t := template.Must(
			template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/"+page),
		)
		srv.tmpls[page] = t
	}

	srv.mux.HandleFunc("GET /health", srv.handleHealth)
	srv.mux.HandleFunc("GET /", srv.handleIndex)
	srv.mux.HandleFunc("GET /categories/{name}", srv.handleCategory)
	srv.mux.HandleFunc("GET /files/{path...}", srv.handleFile)
	srv.mux.HandleFunc("GET /search", srv.handleSearch)
	srv.mux.HandleFunc("GET /chat", srv.handleChatPage)
	srv.mux.HandleFunc("GET /brains", srv.handleBrains)
	srv.mux.HandleFunc("GET /api/brains/{id}", srv.handleBrainDetail)
	srv.mux.HandleFunc("POST /api/search", srv.handleSearchAPI)
	srv.mux.HandleFunc("GET /api/chat/stream", srv.handleChatStream)
	srv.mux.HandleFunc("POST /api/stripe/webhook", srv.handleStripeWebhook)
	srv.mux.Handle("GET /static/", http.FileServerFS(staticFS))

	return srv
}

// WithLogger replaces the Server's slog logger and returns the Server
// for chaining. A nil logger disables the request-logging middleware (the
// mux is exposed unwrapped). Tests use this to inject a buffer-backed
// logger so they can assert the emitted JSON line.
func (s *Server) WithLogger(logger *slog.Logger) *Server {
	s.logger = logger
	return s
}

// Handler returns the http.Handler that should be exposed to a listener.
// The mux is wrapped in loggingMiddleware so every request gets a
// structured JSON log line. Tests should also drive httptest.NewServer
// with Handler() (not s.mux) so the middleware is exercised.
func (s *Server) Handler() http.Handler {
	return loggingMiddleware(s.mux, s.logger)
}

func (s *Server) Serve(ln net.Listener) error {
	return http.Serve(ln, s.Handler())
}
