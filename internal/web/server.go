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
	licenseStore  store.LicenseStore
	embedder      embedder.Embedder
	pipeline      *rag.Pipeline
	cfg           *config.Config
	tmpls         map[string]*template.Template
	mux           *http.ServeMux
	stripeHandler StripeEventHandler
}

// WithLicenseStore turns on paid-tier gating by wiring a LicenseStore into
// the server. The returned Server uses the store both as the source of
// truth for RequirePaid and as the target of the Stripe webhook handler.
// Pass nil to leave the wall down (free-tier-only behaviour).
func (s *Server) WithLicenseStore(licenses store.LicenseStore) *Server {
	s.licenseStore = licenses
	if licenses != nil {
		s.stripeHandler = NewLicenseStripeHandler(licenses)
	}
	return s
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
	// The concrete *SQLiteStore satisfies LicenseStore via the methods
	// added in agentops-012. If the caller passes a Store that does not
	// also implement LicenseStore (e.g. a test double), the licence
	// layer stays disabled and RequirePaid becomes a pass-through.
	if licenses, ok := s.(store.LicenseStore); ok {
		srv.licenseStore = licenses
		srv.stripeHandler = NewLicenseStripeHandler(licenses)
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
	// /api/chat/stream is the paid-tier gated endpoint. RequirePaid is a
	// transparent pass-through when CEREBRA_FREE_TIER_ENABLED is unset or
	// "true" (the default), so local dev keeps working without Stripe.
	// The resolver pattern means WithLicenseStore can be called after
	// NewServer and the route picks up the new store on the next request.
	chatStream := http.HandlerFunc(srv.handleChatStream)
	srv.mux.Handle("GET /api/chat/stream", RequirePaid(func() store.LicenseStore {
		return srv.licenseStore
	})(chatStream))
	srv.mux.HandleFunc("POST /api/stripe/webhook", srv.handleStripeWebhook)
	srv.mux.Handle("GET /static/", http.FileServerFS(staticFS))

	return srv
}

func (s *Server) Serve(ln net.Listener) error {
	return http.Serve(ln, s.mux)
}
