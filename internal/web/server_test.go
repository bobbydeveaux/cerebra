package web

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/config"
	"github.com/bobbydeveaux/cerebra/internal/rag"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

// TestNewServerWiresAllRoutes verifies that NewServer constructs a Server
// whose mux routes every documented endpoint. Each route is hit once with
// the fake store; we only assert that the route is registered (not 404),
// not that the response body is correct — the handler-level tests cover
// that. NewServer itself accounts for ~40 statements of the package and
// is the single function with the highest coverage cost-per-line; this
// test buys us the constructor in one shot.
func TestNewServerWiresAllRoutes(t *testing.T) {
	st := newFakeStore()
	// Seed enough data that the routes which key off store contents (category,
	// file, brain-detail) hit their happy-path 200 branches rather than their
	// own 404 paths — that way a 404 here cleanly indicates the route is
	// missing from the mux, not just "no rows".
	st.categories = []store.CategorySummary{{Name: "code", FileCount: 1, ChunkCount: 1}}
	st.docs["some.go"] = &scanner.Document{ID: "d1", RelPath: "some.go", Language: "go"}
	st.docChunks["some.go"] = []chunker.Chunk{{ID: "c1", Content: "pkg", StartLine: 1, EndLine: 1, Metadata: chunker.ChunkMeta{Path: "some.go"}}}
	st.brainsByID["abc"] = &store.Brain{BrainID: "abc", ProjectKey: "x", Status: "active"}

	emb := &fakeEmbedder{vec: []float32{0.1, 0.2, 0.3, 0.4}}
	cfg := &config.Config{}
	pipeline := rag.NewPipeline(emb, st, cfg)

	srv := NewServer(st, emb, pipeline, cfg)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.mux == nil {
		t.Fatal("Server.mux is nil after NewServer")
	}

	ts := httptest.NewServer(srv.mux)
	defer ts.Close()

	cases := []struct {
		name   string
		method string
		path   string
		// We accept any non-404 status. A 404 here means the route is missing
		// from the mux entirely.
	}{
		{name: "index", method: http.MethodGet, path: "/"},
		{name: "category", method: http.MethodGet, path: "/categories/code"},
		{name: "file", method: http.MethodGet, path: "/files/some.go"},
		{name: "search-page", method: http.MethodGet, path: "/search?q=x"},
		{name: "chat-page", method: http.MethodGet, path: "/chat"},
		{name: "brains-page", method: http.MethodGet, path: "/brains"},
		{name: "brain-detail", method: http.MethodGet, path: "/api/brains/abc"},
		{name: "search-api", method: http.MethodPost, path: "/api/search"},
		{name: "stripe-webhook", method: http.MethodPost, path: "/api/stripe/webhook"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := http.NewRequest(c.method, ts.URL+c.path, nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			resp.Body.Close()

			if resp.StatusCode == http.StatusNotFound {
				t.Errorf("route %s %s returned 404 — likely not registered with the mux",
					c.method, c.path)
			}
		})
	}
}

func TestServerServeRunsAndShutsDownCleanly(t *testing.T) {
	st := newFakeStore()
	emb := &fakeEmbedder{vec: []float32{0.1}}
	cfg := &config.Config{}
	pipeline := rag.NewPipeline(emb, st, cfg)

	srv := NewServer(st, emb, pipeline, cfg)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	// Hit the server to confirm it's actually listening.
	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()

	// Closing the listener returns http.Serve with an error — that's fine,
	// we just want to confirm the goroutine returns.
	ln.Close()
	<-errCh
}
