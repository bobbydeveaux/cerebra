package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/config"
	"github.com/bobbydeveaux/cerebra/internal/embedder"
	"github.com/bobbydeveaux/cerebra/internal/rag"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

// TestHealthEndpointReturns200WithStatusOK drives a GET /health request
// through the full Handler() chain (mux + logging middleware) and asserts
// the documented response shape: 200, JSON content type, and a body of
// {"status":"ok","version":"<buildVersion>"}. The default buildVersion is
// "dev" so that's what we assert on. Production builds override via
// -ldflags and that override is exercised by the version test below.
func TestHealthEndpointReturns200WithStatusOK(t *testing.T) {
	srv := newHealthTestServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if got, want := resp.Header.Get("Content-Type"), "application/json"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}

	var body healthResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want %q", body.Status, "ok")
	}
	if body.Version != "dev" {
		t.Errorf("version = %q, want %q (default buildVersion)", body.Version, "dev")
	}
}

// TestHealthEndpointReportsOverriddenBuildVersion proves that the
// package-level buildVersion var is the source of truth — overriding it
// (as -ldflags does at link time) changes the response. We restore it on
// teardown so neighbouring tests see the default "dev" value.
func TestHealthEndpointReportsOverriddenBuildVersion(t *testing.T) {
	original := buildVersion
	buildVersion = "test-sha-abc123"
	t.Cleanup(func() { buildVersion = original })

	srv := newHealthTestServer()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	var body healthResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Version != "test-sha-abc123" {
		t.Errorf("version = %q, want %q", body.Version, "test-sha-abc123")
	}
}

// TestHealthEndpointDoesNotTouchDependencies wires the server with a
// panicking embedder, then hits /health and confirms no panic. This
// locks in acceptance criterion §4: "Health check does NOT query the
// database, call Stripe, or touch any external service." If a future
// edit accidentally wires the health handler to s.embedder, this test
// fails with status 500 (panic recovered by net/http) or a connection
// reset.
//
// We use the embedder rather than the store as the trip-wire because
// store.Store has ~30 methods; defining a panicking version would be
// noisy. embedder.Embedder has two methods and is a clean canary.
func TestHealthEndpointDoesNotTouchDependencies(t *testing.T) {
	st := newFakeStore()
	emb := &panickingEmbedder{}
	cfg := &config.Config{}

	// Pipeline only stores its dependencies; doesn't call them at construction.
	pipeline := rag.NewPipeline(emb, st, cfg)

	srv := NewServer(st, emb, pipeline, cfg)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d (health handler must not touch embedder)",
			resp.StatusCode, http.StatusOK)
	}
}

// newHealthTestServer constructs a Server with the minimal viable
// dependencies for the routes that don't read state. We use the same
// fakes the logging tests use but seed nothing — /health doesn't read
// from the store, so seeded data is irrelevant noise.
func newHealthTestServer() *Server {
	st := newFakeStore()
	emb := &fakeEmbedder{vec: []float32{0.1}}
	cfg := &config.Config{}
	pipeline := rag.NewPipeline(emb, st, cfg)
	return NewServer(st, emb, pipeline, cfg)
}

// panickingEmbedder implements embedder.Embedder. Every method panics.
// If the health handler ever touches s.embedder, this fake guarantees
// a 5xx response (panic recovered by net/http) rather than a silent
// dependency on a service that may not be available at boot.
type panickingEmbedder struct{}

func (panickingEmbedder) Embed(_ context.Context, _ []string) ([][]float32, error) {
	panic("health handler must not call embedder.Embed")
}

func (panickingEmbedder) Dimensions() int {
	panic("health handler must not call embedder.Dimensions")
}

// Compile-time assertion that panickingEmbedder satisfies the
// embedder.Embedder interface. If the interface gains a method, this
// fails at build time and tells us to update the fake.
var (
	_ embedder.Embedder = panickingEmbedder{}
	_ store.Store       = (*fakeStore)(nil) // confirms our fake remains current
)
