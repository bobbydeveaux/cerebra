package web

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobbydeveaux/cerebra/internal/chunker"
	"github.com/bobbydeveaux/cerebra/internal/scanner"
	"github.com/bobbydeveaux/cerebra/internal/store"
)

// newWikiServer wires a Server with the embedded templates loaded the same
// way NewServer does, but skips the pipeline / embedder / config stack so we
// can test handler rendering against fakeStore.
func newWikiServer(t *testing.T, st store.Store, emb *fakeEmbedder) *Server {
	t.Helper()
	srv := &Server{
		store:    st,
		embedder: emb,
		mux:      http.NewServeMux(),
		tmpls:    make(map[string]*template.Template),
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
	pages := []string{"index.html", "category.html", "document.html", "search.html", "chat.html", "brains.html"}
	for _, page := range pages {
		tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/layout.html", "templates/"+page)
		if err != nil {
			t.Fatalf("parsing template %s: %v", page, err)
		}
		srv.tmpls[page] = tmpl
	}
	return srv
}

func TestHandleIndexRendersPage(t *testing.T) {
	st := newFakeStore()
	st.stats = store.Stats{Repos: 2, Files: 5, Chunks: 99, Categories: 3, LastScan: "2026-05-29", DBSizeMB: 1.5}
	st.categories = []store.CategorySummary{
		{Name: "code", Description: "code files", FileCount: 4, ChunkCount: 88},
	}
	st.files = []store.FileSummary{
		{RelPath: "cmd/main.go", Language: "go", FileType: "code"},
		{RelPath: "internal/store/store.go", Language: "go", FileType: "code"},
		{RelPath: "README.md", Language: "markdown", FileType: "docs"},
	}

	srv := newWikiServer(t, st, &fakeEmbedder{vec: []float32{0, 0, 0, 0}})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	srv.handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Jor-El Knowledge Base") {
		t.Errorf("expected title in body, got: %q", body[:min(len(body), 200)])
	}
	if !strings.Contains(body, "code") {
		t.Errorf("expected category name in body")
	}
}

func TestHandleIndexNonRootReturns404(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/anything-else", nil)
	w := httptest.NewRecorder()

	srv.handleIndex(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestHandleCategoryFound(t *testing.T) {
	st := newFakeStore()
	st.categories = []store.CategorySummary{{Name: "docs", FileCount: 1, ChunkCount: 2}}
	st.filesByCat = map[string][]store.FileSummary{
		"docs": {
			{RelPath: "docs/HLD.md", Language: "markdown"},
			{RelPath: "docs/PRD.md", Language: "markdown"},
			{RelPath: "README.md", Language: "markdown"},
		},
	}
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/categories/docs", nil)
	req.SetPathValue("name", "docs")
	w := httptest.NewRecorder()

	srv.handleCategory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "docs") {
		t.Errorf("expected category name in body")
	}
}

func TestHandleCategoryNotFound(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/categories/nope", nil)
	req.SetPathValue("name", "nope")
	w := httptest.NewRecorder()

	srv.handleCategory(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestHandleFileFound(t *testing.T) {
	st := newFakeStore()
	st.docs["cmd/main.go"] = &scanner.Document{ID: "doc-1", Path: "/abs/cmd/main.go", RelPath: "cmd/main.go", Language: "go"}
	st.docChunks["cmd/main.go"] = []chunker.Chunk{
		{ID: "c1", Content: "package main", StartLine: 1, EndLine: 1, Metadata: chunker.ChunkMeta{Path: "cmd/main.go", Language: "go"}},
	}
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/files/cmd/main.go", nil)
	req.SetPathValue("path", "cmd/main.go")
	w := httptest.NewRecorder()

	srv.handleFile(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestHandleFileNotFound(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/files/missing.go", nil)
	req.SetPathValue("path", "missing.go")
	w := httptest.NewRecorder()

	srv.handleFile(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
}

func TestHandleSearchPageRenders(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/search?q=foo", nil)
	w := httptest.NewRecorder()

	srv.handleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Search") {
		t.Errorf("expected search page title in body")
	}
}

func TestHandleSearchAPIMissingQuery(t *testing.T) {
	st := newFakeStore()
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.handleSearchAPI(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestHandleSearchAPINoResults(t *testing.T) {
	st := newFakeStore() // empty searchResults + ftsResults
	srv := newWikiServer(t, st, &fakeEmbedder{vec: []float32{0.1, 0.2, 0.3, 0.4}})

	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader("q=anything"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.handleSearchAPI(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "No results found") {
		t.Errorf("expected no-results message, got %q", w.Body.String())
	}
}

func TestHandleSearchAPIRendersVectorResults(t *testing.T) {
	st := newFakeStore()
	st.searchResults = []store.SearchResult{
		{
			Chunk: chunker.Chunk{
				ID:        "c1",
				Content:   "func add(a, b int) int { return a + b }",
				StartLine: 5,
				EndLine:   7,
				Metadata: chunker.ChunkMeta{
					Path:     "math/add.go",
					Repo:     "mathlib",
					Category: scanner.Category("code"),
					Language: "go",
				},
			},
			Score: 0.87,
		},
	}
	srv := newWikiServer(t, st, &fakeEmbedder{vec: []float32{0.1, 0.2, 0.3, 0.4}})

	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader("q=add"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.handleSearchAPI(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"math/add.go", "0.87", ":5-7", "mathlib", "func add"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q. body=%s", want, body)
		}
	}
}

func TestHandleSearchAPIFallsBackToFTSWhenEmbedderFails(t *testing.T) {
	st := newFakeStore()
	st.ftsResults = []store.SearchResult{
		{
			Chunk: chunker.Chunk{
				Content:  "FTS hit",
				Metadata: chunker.ChunkMeta{Path: "fts/x.md"},
			},
			Score: 0.5,
		},
	}
	emb := &fakeEmbedder{vec: []float32{0.1, 0.2, 0.3, 0.4}, embedErr: io.ErrUnexpectedEOF}
	srv := newWikiServer(t, st, emb)

	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader("q=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.handleSearchAPI(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "fts/x.md") {
		t.Errorf("expected FTS fallback result, got %q", w.Body.String())
	}
}

func TestHandleSearchAPITruncatesLongContent(t *testing.T) {
	st := newFakeStore()
	long := strings.Repeat("a", 400)
	st.searchResults = []store.SearchResult{
		{
			Chunk: chunker.Chunk{
				Content:  long,
				Metadata: chunker.ChunkMeta{Path: "long.go"},
			},
			Score: 0.1,
		},
	}
	srv := newWikiServer(t, st, &fakeEmbedder{vec: []float32{0.1}})

	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader("q=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.handleSearchAPI(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "...") {
		t.Errorf("expected ellipsis truncation marker in body")
	}
}

func TestBuildTopDirs(t *testing.T) {
	files := []store.FileSummary{
		{RelPath: "cmd/main.go"},
		{RelPath: "cmd/sub/foo.go"},
		{RelPath: "internal/web/server.go"},
		{RelPath: "README.md"},
	}
	dirs := buildTopDirs(files)
	if len(dirs) != 3 {
		t.Fatalf("want 3 top dirs, got %d (%v)", len(dirs), dirs)
	}
	if dirs[0].Name != "cmd" || dirs[0].FileCount != 2 {
		t.Errorf("expected cmd to be first with count 2, got %+v", dirs[0])
	}
	// Bare files at the top of the relative tree become "."
	foundRoot := false
	for _, d := range dirs {
		if d.Name == "." {
			foundRoot = true
		}
	}
	if !foundRoot {
		t.Errorf("expected root '.' bucket for top-level README, got %+v", dirs)
	}
}

func TestBuildDirTree(t *testing.T) {
	files := []store.FileSummary{
		{RelPath: "cmd/main.go", Language: "go"},
		{RelPath: "cmd/sub/foo.go", Language: "go"},
	}
	tree := buildDirTree(files)
	if len(tree) != 1 {
		t.Fatalf("want 1 root entry (cmd), got %d", len(tree))
	}
	if tree[0].Name != "cmd" || !tree[0].IsDir {
		t.Errorf("expected cmd dir entry, got %+v", tree[0])
	}
	if len(tree[0].Children) != 2 {
		t.Errorf("want 2 children of cmd, got %d", len(tree[0].Children))
	}
}

func TestItoa(t *testing.T) {
	cases := map[int]string{
		0:    "0",
		1:    "1",
		9:    "9",
		10:   "10",
		123:  "123",
		1000: "1000",
	}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestTemplateEscape(t *testing.T) {
	cases := map[string]string{
		"":                  "",
		"plain":             "plain",
		"<script>":          "&lt;script&gt;",
		"a & b":             "a &amp; b",
		`he said "hi"`:      "he said &quot;hi&quot;",
		"<a href=\"x\">&y": "&lt;a href=&quot;x&quot;&gt;&amp;y",
	}
	for in, want := range cases {
		if got := template_escape(in); got != want {
			t.Errorf("template_escape(%q) = %q, want %q", in, got, want)
		}
	}
}

// --- agentops-117: additional branch coverage ---

// TestHandleSearchAPIEmbedderEmptyFallsBackToFTS exercises the fallback that
// fires when the embedder succeeds but vector Search returns no rows. This is
// distinct from the embedErr path: here Embed returns a vector cleanly, yet
// len(results)==0 still routes through SearchFTS.
func TestHandleSearchAPIEmbedderEmptyFallsBackToFTS(t *testing.T) {
	st := newFakeStore() // searchResults empty -> vector search yields nothing
	st.ftsResults = []store.SearchResult{
		{
			Chunk: chunker.Chunk{
				Content:  "fts only hit",
				Metadata: chunker.ChunkMeta{Path: "fts/only.md"},
			},
			Score: 0.42,
		},
	}
	emb := &fakeEmbedder{vec: []float32{0.1, 0.2, 0.3, 0.4}} // embed succeeds
	srv := newWikiServer(t, st, emb)

	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader("q=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.handleSearchAPI(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "fts/only.md") {
		t.Errorf("expected FTS fallback result after empty vector search, got %q", body)
	}
}

// TestHandleSearchAPIOmitsLineSpanAndMeta covers the two conditional render
// branches inside handleSearchAPI: StartLine==0 suppresses the result-lines
// span, and an empty Repo suppresses the result-meta div. The file link and
// score must still render.
func TestHandleSearchAPIOmitsLineSpanAndMeta(t *testing.T) {
	st := newFakeStore()
	st.searchResults = []store.SearchResult{
		{
			Chunk: chunker.Chunk{
				Content:   "no line span, no repo",
				StartLine: 0,
				EndLine:   0,
				Metadata:  chunker.ChunkMeta{Path: "bare/file.txt"}, // Repo empty
			},
			Score: 0.33,
		},
	}
	srv := newWikiServer(t, st, &fakeEmbedder{vec: []float32{0.1, 0.2, 0.3, 0.4}})

	req := httptest.NewRequest(http.MethodPost, "/api/search", strings.NewReader("q=bare"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.handleSearchAPI(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "bare/file.txt") {
		t.Errorf("expected file link in body, got %q", body)
	}
	if !strings.Contains(body, "0.33") {
		t.Errorf("expected score in body, got %q", body)
	}
	if strings.Contains(body, "result-lines") {
		t.Errorf("StartLine==0 must suppress result-lines span, got %q", body)
	}
	if strings.Contains(body, "result-meta") {
		t.Errorf("empty Repo must suppress result-meta div, got %q", body)
	}
}

// TestHandleCategoryGroupsAndSortsDirs covers handleCategory grouping files
// into DirGroups keyed by directory and emitting them in sorted directory
// order. Two distinct directories must both render.
func TestHandleCategoryGroupsAndSortsDirs(t *testing.T) {
	st := newFakeStore()
	st.categories = []store.CategorySummary{{Name: "code", FileCount: 3, ChunkCount: 6}}
	st.filesByCat = map[string][]store.FileSummary{
		"code": {
			{RelPath: "zeta/z.go", Language: "go"},
			{RelPath: "alpha/a.go", Language: "go"},
			{RelPath: "alpha/b.go", Language: "go"},
		},
	}
	srv := newWikiServer(t, st, &fakeEmbedder{})

	req := httptest.NewRequest(http.MethodGet, "/categories/code", nil)
	req.SetPathValue("name", "code")
	w := httptest.NewRecorder()

	srv.handleCategory(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	ai := strings.Index(body, "alpha")
	zi := strings.Index(body, "zeta")
	if ai < 0 || zi < 0 {
		t.Fatalf("expected both alpha and zeta directories rendered, got %q", body)
	}
	if ai > zi {
		t.Errorf("expected alpha to render before zeta (sorted dir order), alpha=%d zeta=%d", ai, zi)
	}
}

// TestBuildTopDirsEmpty guards the nil-input path: no files yields no dirs and
// must not panic.
func TestBuildTopDirsEmpty(t *testing.T) {
	if got := buildTopDirs(nil); len(got) != 0 {
		t.Errorf("buildTopDirs(nil) = %v, want empty", got)
	}
}

// TestBuildDirTreeEmpty guards the nil-input path for the tree builder.
func TestBuildDirTreeEmpty(t *testing.T) {
	if got := buildDirTree(nil); len(got) != 0 {
		t.Errorf("buildDirTree(nil) = %v, want empty", got)
	}
}
