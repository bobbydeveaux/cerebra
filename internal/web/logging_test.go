package web

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/config"
	"github.com/bobbydeveaux/cerebra/internal/rag"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

// safeBuffer wraps bytes.Buffer with a mutex so it's safe for the slog
// JSON handler to write to concurrently with the test reading from it.
// slog itself takes an internal lock around writes, but the test reads
// after the response has returned so concurrent access only matters when
// running -race; the mutex keeps that clean.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// newTestServer constructs a Server with a buffer-backed JSON logger and
// returns both the server and the buffer so tests can read the emitted
// log lines.
func newTestServer(t *testing.T) (*Server, *safeBuffer) {
	t.Helper()
	st := newFakeStore()
	st.categories = []store.CategorySummary{{Name: "code", FileCount: 1, ChunkCount: 1}}
	st.docs["some.go"] = &scanner.Document{ID: "d1", RelPath: "some.go", Language: "go"}
	st.docChunks["some.go"] = []chunker.Chunk{{ID: "c1", Content: "pkg", StartLine: 1, EndLine: 1, Metadata: chunker.ChunkMeta{Path: "some.go"}}}
	emb := &fakeEmbedder{vec: []float32{0.1, 0.2, 0.3, 0.4}}
	cfg := &config.Config{}
	pipeline := rag.NewPipeline(emb, st, cfg)

	srv := NewServer(st, emb, pipeline, cfg)
	buf := &safeBuffer{}
	srv.WithLogger(slog.New(slog.NewJSONHandler(buf, nil)))
	return srv, buf
}

// TestLoggingMiddlewareEmitsJSONLine drives a known route through the
// full Handler() chain and asserts that exactly one JSON line is emitted
// with the documented fields. Covers the happy 200 path.
func TestLoggingMiddlewareEmitsJSONLine(t *testing.T) {
	srv, buf := newTestServer(t)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("no log line emitted")
	}
	// One request -> exactly one JSON line.
	if strings.Count(line, "\n") != 0 {
		t.Errorf("expected exactly one log line, got: %q", line)
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("log line is not JSON: %v\nline: %s", err, line)
	}

	checkString(t, entry, "msg", "http_request")
	checkString(t, entry, "method", http.MethodGet)
	checkString(t, entry, "path", "/")

	status, ok := entry["status_code"].(float64)
	if !ok {
		t.Fatalf("status_code missing or wrong type: %v (%T)", entry["status_code"], entry["status_code"])
	}
	if int(status) != http.StatusOK {
		t.Errorf("status_code = %d, want %d", int(status), http.StatusOK)
	}

	if _, ok := entry["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms missing or wrong type: %v (%T)", entry["duration_ms"], entry["duration_ms"])
	}

	if _, present := entry["error"]; present {
		t.Errorf("error field should be omitted on success path, got: %v", entry["error"])
	}
}

// TestLoggingMiddlewareRecords404 verifies the recorder captures non-200
// status codes when an upstream handler calls http.NotFound. handleFile
// returns 404 for an unknown path.
func TestLoggingMiddlewareRecords404(t *testing.T) {
	srv, buf := newTestServer(t)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/files/does-not-exist")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()

	line := strings.TrimSpace(buf.String())
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("log line is not JSON: %v\nline: %s", err, line)
	}

	status, _ := entry["status_code"].(float64)
	if int(status) != http.StatusNotFound {
		t.Errorf("status_code = %d, want %d", int(status), http.StatusNotFound)
	}
	checkString(t, entry, "path", "/files/does-not-exist")
	checkString(t, entry, "method", http.MethodGet)
}

// TestLoggingMiddlewareNilLoggerIsPassthrough ensures that supplying a
// nil logger disables the middleware (mux is exposed unwrapped). This is
// the safety hatch — if logging breaks production, ops can flip to nil
// without losing routing.
func TestLoggingMiddlewareNilLoggerIsPassthrough(t *testing.T) {
	st := newFakeStore()
	emb := &fakeEmbedder{vec: []float32{0.1}}
	cfg := &config.Config{}
	pipeline := rag.NewPipeline(emb, st, cfg)
	srv := NewServer(st, emb, pipeline, cfg).WithLogger(nil)

	h := srv.Handler()
	// With a nil logger the middleware returns next directly, so Handler()
	// should be the same *http.ServeMux instance.
	if h != srv.mux {
		t.Errorf("Handler() with nil logger should return s.mux unwrapped")
	}
}

// TestStatusRecorderDefaults200 covers the implicit-200 case: when an
// upstream handler writes a body without calling WriteHeader first, the
// recorder must still report 200 (matching what net/http actually sends).
func TestStatusRecorderDefaults200(t *testing.T) {
	rw := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: rw, status: http.StatusOK}
	if _, err := rec.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if rec.status != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.status, http.StatusOK)
	}
	if !rec.wroteHeader {
		t.Error("wroteHeader should be true after Write")
	}
}

// TestStatusRecorderWriteHeaderOnce verifies the recorder honours the
// net/http convention that only the first WriteHeader call takes effect.
func TestStatusRecorderWriteHeaderOnce(t *testing.T) {
	rw := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: rw, status: http.StatusOK}
	rec.WriteHeader(http.StatusTeapot)
	rec.WriteHeader(http.StatusInternalServerError)
	if rec.status != http.StatusTeapot {
		t.Errorf("status = %d, want %d (subsequent WriteHeader calls should be ignored)", rec.status, http.StatusTeapot)
	}
}

// TestStatusRecorderFlushForwarded verifies the recorder forwards Flush
// to the underlying ResponseWriter when it implements http.Flusher.
// Streaming handlers like the chat SSE endpoint depend on this.
func TestStatusRecorderFlushForwarded(t *testing.T) {
	rw := httptest.NewRecorder()
	rec := &statusRecorder{ResponseWriter: rw, status: http.StatusOK}
	// httptest.ResponseRecorder implements http.Flusher; the call must
	// not panic and must reach the underlying recorder. We assert by
	// flushing after writing a body and reading the buffered output.
	rec.WriteHeader(http.StatusOK)
	if _, err := rec.Write([]byte("chunk")); err != nil {
		t.Fatalf("write: %v", err)
	}
	rec.Flush()
	if rw.Body.String() != "chunk" {
		t.Errorf("body = %q, want %q", rw.Body.String(), "chunk")
	}
}

// checkString is a small helper to assert that a JSON map entry equals
// an expected string. Keeps the table-style tests above readable.
func checkString(t *testing.T, m map[string]any, key, want string) {
	t.Helper()
	got, ok := m[key].(string)
	if !ok {
		t.Fatalf("%s missing or wrong type: %v (%T)", key, m[key], m[key])
	}
	if got != want {
		t.Errorf("%s = %q, want %q", key, got, want)
	}
}
