package web

import (
	"embed"
	"html/template"
	"net"
	"net/http"

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

func (s *Server) Serve(ln net.Listener) error {
	return http.Serve(ln, s.mux)
}
