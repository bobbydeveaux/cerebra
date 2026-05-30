package confluence

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobbydeveaux/cerebra/internal/scanner"
)

// ---------- New ----------

func TestNew_TrimsTrailingSlashAndStoresFields(t *testing.T) {
	p := New("https://example.atlassian.net/wiki/", "u@example.com", "tok", []string{"ENG", "OPS"})
	if p == nil {
		t.Fatal("New returned nil")
	}
	if p.baseURL != "https://example.atlassian.net/wiki" {
		t.Errorf("baseURL = %q, want trailing slash trimmed", p.baseURL)
	}
	if p.email != "u@example.com" {
		t.Errorf("email = %q", p.email)
	}
	if p.apiToken != "tok" {
		t.Errorf("apiToken = %q", p.apiToken)
	}
	if len(p.spaces) != 2 || p.spaces[0] != "ENG" || p.spaces[1] != "OPS" {
		t.Errorf("spaces = %v", p.spaces)
	}
	if p.client == nil {
		t.Fatal("http client is nil")
	}
	if p.client.Timeout != 30*time.Second {
		t.Errorf("client.Timeout = %v", p.client.Timeout)
	}
}

func TestNew_NoTrailingSlashLeftAlone(t *testing.T) {
	p := New("https://example.atlassian.net/wiki", "u", "t", nil)
	if p.baseURL != "https://example.atlassian.net/wiki" {
		t.Errorf("baseURL = %q", p.baseURL)
	}
	if p.spaces != nil && len(p.spaces) != 0 {
		t.Errorf("expected empty/nil spaces, got %v", p.spaces)
	}
}

// ---------- sanitisePath ----------

func TestSanitisePath(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"simple lowercase", "hello", "hello"},
		{"mixed case", "Hello World", "hello-world"},
		{"multi punctuation collapse", "Hello!!! World???", "hello-world"},
		{"leading and trailing strip", "  hello  ", "hello"},
		{"all punctuation becomes empty", "!!!", ""},
		{"numeric kept", "Q3 2026 plan", "q3-2026-plan"},
		{"unicode replaced", "café", "caf"},
		{"long string truncated to 100 chars", strings.Repeat("a", 150), strings.Repeat("a", 100)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitisePath(tt.title)
			if got != tt.want {
				t.Errorf("sanitisePath(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

// ---------- htmlToText ----------

func TestHTMLToText_Empty(t *testing.T) {
	if got := htmlToText(""); got != "" {
		t.Errorf("htmlToText(\"\") = %q, want empty", got)
	}
}

func TestHTMLToText_Headings(t *testing.T) {
	in := "<h1>One</h1><h2>Two</h2><h3>Three</h3><h4>Four</h4><h5>Five</h5><h6>Six</h6>"
	out := htmlToText(in)
	for i, name := range []string{"One", "Two", "Three", "Four", "Five", "Six"} {
		want := strings.Repeat("#", i+1) + " " + name
		if !strings.Contains(out, want) {
			t.Errorf("htmlToText output missing %q\nfull output: %q", want, out)
		}
	}
}

func TestHTMLToText_Lists(t *testing.T) {
	in := "<ul><li>alpha</li><li>beta</li></ul>"
	out := htmlToText(in)
	if !strings.Contains(out, "- alpha") || !strings.Contains(out, "- beta") {
		t.Errorf("list items not converted: %q", out)
	}
}

func TestHTMLToText_ParagraphsAndBreaks(t *testing.T) {
	in := "<p>first</p><p>second</p>line1<br/>line2"
	out := htmlToText(in)
	if !strings.Contains(out, "first") || !strings.Contains(out, "second") {
		t.Errorf("paragraph text missing: %q", out)
	}
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
		t.Errorf("br-separated text missing: %q", out)
	}
}

func TestHTMLToText_CodeBlockMacro(t *testing.T) {
	in := `before<ac:structured-macro ac:name="code"><ac:plain-text-body><![CDATA[fmt.Println("hi")]]></ac:plain-text-body></ac:structured-macro>after`
	out := htmlToText(in)
	if !strings.Contains(out, "```") {
		t.Errorf("code fences missing: %q", out)
	}
	if !strings.Contains(out, `fmt.Println("hi")`) {
		t.Errorf("code body missing: %q", out)
	}
}

func TestHTMLToText_Tables(t *testing.T) {
	in := "<table><tr><th>k</th><th>v</th></tr><tr><td>a</td><td>1</td></tr></table>"
	out := htmlToText(in)
	if !strings.Contains(out, "| k") || !strings.Contains(out, "| v") {
		t.Errorf("table header cells missing: %q", out)
	}
	if !strings.Contains(out, "| a") || !strings.Contains(out, "| 1") {
		t.Errorf("table data cells missing: %q", out)
	}
}

func TestHTMLToText_Links(t *testing.T) {
	in := `<a href="https://example.com">click</a>`
	out := htmlToText(in)
	if !strings.Contains(out, "click (https://example.com)") {
		t.Errorf("link not normalised: %q", out)
	}
}

func TestHTMLToText_BoldItalicCode(t *testing.T) {
	in := "<strong>bold</strong> <em>italic</em> <code>inline</code>"
	out := htmlToText(in)
	for _, want := range []string{"**bold**", "*italic*", "`inline`"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

func TestHTMLToText_GenericMacroStripped(t *testing.T) {
	in := `prefix<ac:structured-macro ac:name="info"><ac:rich-text-body>this is dropped</ac:rich-text-body></ac:structured-macro>suffix`
	out := htmlToText(in)
	if strings.Contains(out, "this is dropped") {
		t.Errorf("macro body should be removed: %q", out)
	}
	if !strings.Contains(out, "prefix") || !strings.Contains(out, "suffix") {
		t.Errorf("surrounding text lost: %q", out)
	}
}

func TestHTMLToText_RawTagStrip(t *testing.T) {
	in := `<div class="x"><span>hi</span></div>`
	out := htmlToText(in)
	if strings.Contains(out, "<") || strings.Contains(out, ">") {
		t.Errorf("tags not stripped: %q", out)
	}
	if !strings.Contains(out, "hi") {
		t.Errorf("body text missing: %q", out)
	}
}

func TestHTMLToText_EntityDecoding(t *testing.T) {
	in := "&amp;&lt;&gt;&quot;&#39;&nbsp;&rarr;&larr;&rsquo;&lsquo;&rdquo;&ldquo;&mdash;&ndash;&hellip;"
	out := htmlToText(in)
	wants := []string{"&", "<", ">", "\"", "'", " ", "->", "<-", "'", "'", "\"", "\"", " -- ", "-", "..."}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("entity replacement missing %q in %q", w, out)
		}
	}
}

func TestHTMLToText_WhitespaceCollapse(t *testing.T) {
	in := "<p>a</p>\n\n\n\n\n<p>b</p>"
	out := htmlToText(in)
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("excessive newlines not collapsed: %q", out)
	}
}

// ---------- pageToDocument ----------

func TestPageToDocument_BasicMapping(t *testing.T) {
	p := New("https://example.atlassian.net/wiki", "u", "t", nil)
	pg := v1Page{
		ID:    "12345",
		Type:  "page",
		Title: "Hello / World",
	}
	pg.Space.Key = "ENG"
	pg.Space.Name = "Engineering"
	pg.Body.Storage.Value = "<p>hello</p>"
	pg.Version.Number = 4
	pg.Version.When = "2026-05-29T10:00:00Z"
	pg.Version.By.DisplayName = "Alice"
	pg.Version.By.Email = "alice@example.com"
	pg.Links.WebUI = "/spaces/ENG/pages/12345"
	pg.Links.Self = "https://example.atlassian.net/wiki/rest/api/content/12345"

	sp := v1Space{Key: "ENG", Name: "Engineering"}

	doc := p.pageToDocument(pg, sp)

	if doc.ID == "" {
		t.Error("Document ID empty")
	}
	if doc.Path != "https://example.atlassian.net/wiki/spaces/ENG/pages/12345" {
		t.Errorf("Path = %q", doc.Path)
	}
	if doc.RelPath == "" || !strings.Contains(doc.RelPath, "confluence/ENG/") {
		t.Errorf("RelPath = %q", doc.RelPath)
	}
	if doc.Repo != "confluence/ENG" {
		t.Errorf("Repo = %q", doc.Repo)
	}
	if doc.Category != scanner.CategoryDocs {
		t.Errorf("Category = %q", doc.Category)
	}
	if doc.FileType != scanner.FileTypeWiki {
		t.Errorf("FileType = %q", doc.FileType)
	}
	if doc.SourceType != scanner.SourceTypeConfluence {
		t.Errorf("SourceType = %q", doc.SourceType)
	}
	if !strings.Contains(doc.Content, "# Hello / World") {
		t.Errorf("Content lacks title heading: %q", doc.Content)
	}
	if !strings.Contains(doc.Content, "hello") {
		t.Errorf("Content lacks body: %q", doc.Content)
	}
	if doc.ContentHash == "" {
		t.Error("ContentHash empty")
	}
	expectedTime, _ := time.Parse(time.RFC3339, "2026-05-29T10:00:00Z")
	if !doc.ModTime.Equal(expectedTime) {
		t.Errorf("ModTime = %v, want %v", doc.ModTime, expectedTime)
	}
	for k, v := range map[string]string{
		"source":     "confluence",
		"space_key":  "ENG",
		"space_name": "Engineering",
		"page_id":    "12345",
		"page_title": "Hello / World",
		"version":    "4",
		"web_url":    "https://example.atlassian.net/wiki/spaces/ENG/pages/12345",
		"author":     "Alice",
	} {
		if got := doc.Metadata[k]; got != v {
			t.Errorf("Metadata[%q] = %q, want %q", k, got, v)
		}
	}
}

func TestPageToDocument_ModTimeFallbackWhenWhenMissingOrBad(t *testing.T) {
	p := New("https://example.atlassian.net/wiki", "u", "t", nil)

	cases := []struct {
		name string
		when string
	}{
		{"empty", ""},
		{"invalid", "not-a-real-timestamp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pg := v1Page{Title: "t", ID: "1"}
			pg.Version.When = tc.when
			pg.Links.WebUI = "/x"
			sp := v1Space{Key: "S"}

			before := time.Now()
			doc := p.pageToDocument(pg, sp)
			after := time.Now()

			if doc.ModTime.Before(before.Add(-1*time.Second)) || doc.ModTime.After(after.Add(1*time.Second)) {
				t.Errorf("ModTime %v not within [%v, %v]", doc.ModTime, before, after)
			}
		})
	}
}

func TestPageToDocument_HashIsContentDeterministic(t *testing.T) {
	p := New("https://example.atlassian.net/wiki", "u", "t", nil)
	pg := v1Page{ID: "1", Title: "Same"}
	pg.Body.Storage.Value = "<p>same body</p>"
	pg.Links.WebUI = "/x"
	sp := v1Space{Key: "S"}

	d1 := p.pageToDocument(pg, sp)
	d2 := p.pageToDocument(pg, sp)
	if d1.ContentHash != d2.ContentHash {
		t.Errorf("hash mismatch: %s vs %s", d1.ContentHash, d2.ContentHash)
	}

	pg.Body.Storage.Value = "<p>different body</p>"
	d3 := p.pageToDocument(pg, sp)
	if d1.ContentHash == d3.ContentHash {
		t.Errorf("hash should change when body changes")
	}
}

// ---------- get error paths ----------

func TestGet_Non200ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden body"))
	}))
	defer srv.Close()

	p := New(srv.URL, "u", "t", nil)
	var out struct{}
	err := p.get(context.Background(), "/anything", &out)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention 403: %v", err)
	}
}

func TestGet_BasicAuthHeaderSent(t *testing.T) {
	var gotUser, gotPass string
	var ok bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, ok = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	p := New(srv.URL, "alice@example.com", "secret-token", nil)
	var out struct{}
	if err := p.get(context.Background(), "/foo", &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || gotUser != "alice@example.com" || gotPass != "secret-token" {
		t.Errorf("basic auth = (%q,%q,ok=%v)", gotUser, gotPass, ok)
	}
}

func TestGet_AbsoluteURLPassedThrough(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	// Use a different baseURL, but pass an absolute URL pointing at srv. The
	// absolute path should be honoured rather than concatenated.
	p := New("https://wrong.example.com/wiki", "u", "t", nil)
	var out struct{}
	if err := p.get(context.Background(), srv.URL+"/abs", &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hits != 1 {
		t.Errorf("expected 1 hit on absolute URL, got %d", hits)
	}
}

func TestGet_ContextCanceledReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	p := New(srv.URL, "u", "t", nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out struct{}
	err := p.get(ctx, "/slow", &out)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

// ---------- end-to-end Scan ----------

// fakeServer constructs an httptest server that mimics the v1 REST API for one
// space + one page. Pagination links (`_links.next`) work by returning an
// absolute URL pointing back at this same server with a marker query param.
type fakeServer struct {
	mu      sync.Mutex
	pages   []v1Page
	spaces  []v1Space
	hits    int
	failGet bool
}

func newFakeServer() *fakeServer {
	fs := &fakeServer{
		spaces: []v1Space{
			{ID: 1, Key: "ENG", Name: "Engineering", Type: "global", Status: "current"},
			{ID: 2, Key: "OPS", Name: "Operations", Type: "global", Status: "current"},
		},
		pages: []v1Page{
			func() v1Page {
				pg := v1Page{ID: "100", Type: "page", Status: "current", Title: "First Page"}
				pg.Space.Key = "ENG"
				pg.Space.Name = "Engineering"
				pg.Body.Storage.Value = "<p>first body</p>"
				pg.Version.Number = 1
				pg.Version.When = "2026-05-29T10:00:00Z"
				pg.Version.By.DisplayName = "Alice"
				pg.Links.WebUI = "/spaces/ENG/pages/100"
				return pg
			}(),
			func() v1Page {
				pg := v1Page{ID: "101", Type: "page", Status: "current", Title: "Second Page"}
				pg.Space.Key = "ENG"
				pg.Space.Name = "Engineering"
				pg.Body.Storage.Value = "<p>second body</p>"
				pg.Version.Number = 1
				pg.Links.WebUI = "/spaces/ENG/pages/101"
				return pg
			}(),
			func() v1Page {
				pg := v1Page{ID: "102", Type: "page", Status: "draft", Title: "Draft Page"}
				pg.Space.Key = "ENG"
				pg.Space.Name = "Engineering"
				pg.Body.Storage.Value = "<p>draft body</p>"
				pg.Version.Number = 1
				pg.Links.WebUI = "/spaces/ENG/pages/102"
				return pg
			}(),
		},
	}
	return fs
}

func (f *fakeServer) handler(host string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/rest/api/space", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits++
		failed := f.failGet
		f.mu.Unlock()
		if failed {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Paginate: first call returns spaces[0:1] with next link, second call
		// returns spaces[1:] without a next link.
		if r.URL.Query().Get("cursor") == "" {
			resp := v1SpaceListResponse{
				Results: f.spaces[:1],
				Start:   0,
				Limit:   1,
				Size:    1,
			}
			resp.Links.Next = host + "/rest/api/space?cursor=next"
			writeJSON(w, resp)
			return
		}
		resp := v1SpaceListResponse{
			Results: f.spaces[1:],
			Start:   1,
			Limit:   50,
			Size:    1,
		}
		writeJSON(w, resp)
	})

	mux.HandleFunc("/rest/api/content", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.hits++
		f.mu.Unlock()

		spaceKey := r.URL.Query().Get("spaceKey")
		var matching []v1Page
		for _, pg := range f.pages {
			if pg.Space.Key == spaceKey {
				matching = append(matching, pg)
			}
		}
		resp := v1PageListResponse{Results: matching, Start: 0, Limit: 50, Size: len(matching)}
		writeJSON(w, resp)
	})

	return mux
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestScan_PaginatesSpacesAndFiltersDraftPages(t *testing.T) {
	fs := newFakeServer()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.handler(srv.URL).ServeHTTP(w, r)
	}))
	defer srv.Close()

	p := New(srv.URL, "u@example.com", "tok", nil)

	docs, errs := p.Scan(context.Background())
	collected, collectedErrs := drain(t, docs, errs, 2*time.Second)

	if len(collectedErrs) != 0 {
		t.Errorf("unexpected errors: %v", collectedErrs)
	}
	// Only 2 current pages exist (1 draft filtered).
	if len(collected) != 2 {
		t.Errorf("doc count = %d, want 2; titles=%v", len(collected), titles(collected))
	}
	for _, d := range collected {
		if d.SourceType != scanner.SourceTypeConfluence {
			t.Errorf("doc %q: SourceType = %q", d.Path, d.SourceType)
		}
	}
}

func TestScan_FilterSpacesByKey(t *testing.T) {
	fs := newFakeServer()
	// Add an OPS-space page so we can verify the filter excludes it.
	fs.pages = append(fs.pages, func() v1Page {
		pg := v1Page{ID: "200", Type: "page", Status: "current", Title: "Ops page"}
		pg.Space.Key = "OPS"
		pg.Body.Storage.Value = "<p>ops body</p>"
		pg.Links.WebUI = "/spaces/OPS/pages/200"
		return pg
	}())

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.handler(srv.URL).ServeHTTP(w, r)
	}))
	defer srv.Close()

	// Restrict to OPS only.
	p := New(srv.URL, "u", "t", []string{"ops"}) // case-insensitive filter

	docs, errs := p.Scan(context.Background())
	collected, collectedErrs := drain(t, docs, errs, 2*time.Second)
	if len(collectedErrs) != 0 {
		t.Errorf("unexpected errs: %v", collectedErrs)
	}
	if len(collected) != 1 {
		t.Fatalf("expected 1 OPS doc, got %d (%v)", len(collected), titles(collected))
	}
	if collected[0].Repo != "confluence/OPS" {
		t.Errorf("Repo = %q, want confluence/OPS", collected[0].Repo)
	}
}

func TestScan_SpaceListErrorYieldsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := New(srv.URL, "u", "t", nil)
	docs, errs := p.Scan(context.Background())
	collected, collectedErrs := drain(t, docs, errs, 2*time.Second)

	if len(collected) != 0 {
		t.Errorf("expected 0 docs on error, got %d", len(collected))
	}
	if len(collectedErrs) == 0 {
		t.Fatal("expected at least one error on space-list failure")
	}
	if !strings.Contains(collectedErrs[0].Error(), "listing spaces") {
		t.Errorf("first error = %v, want 'listing spaces:' prefix", collectedErrs[0])
	}
}

func TestScan_ContextCancellationStopsScan(t *testing.T) {
	// Server that hangs on /content so we can cancel context mid-scan.
	hangCh := make(chan struct{})
	defer close(hangCh)

	mux := http.NewServeMux()
	mux.HandleFunc("/rest/api/space", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, v1SpaceListResponse{Results: []v1Space{{Key: "ENG"}}, Size: 1})
	})
	mux.HandleFunc("/rest/api/content", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-hangCh:
			return
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := New(srv.URL, "u", "t", nil)
	ctx, cancel := context.WithCancel(context.Background())
	docs, errs := p.Scan(ctx)
	cancel()

	collected, _ := drain(t, docs, errs, 2*time.Second)
	if len(collected) != 0 {
		t.Errorf("expected 0 docs after cancellation, got %d", len(collected))
	}
}

// drain collects all docs and errors from the channels until both close or the
// timeout fires.
func drain(t *testing.T, docs <-chan scanner.Document, errs <-chan error, timeout time.Duration) ([]scanner.Document, []error) {
	t.Helper()
	var (
		gotDocs []scanner.Document
		gotErrs []error
	)
	deadline := time.After(timeout)
	for docs != nil || errs != nil {
		select {
		case d, ok := <-docs:
			if !ok {
				docs = nil
				continue
			}
			gotDocs = append(gotDocs, d)
		case e, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			gotErrs = append(gotErrs, e)
		case <-deadline:
			t.Logf("drain timed out after %v", timeout)
			return gotDocs, gotErrs
		}
	}
	return gotDocs, gotErrs
}

func titles(docs []scanner.Document) []string {
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, fmt.Sprintf("%s/%s", d.Repo, d.Metadata["page_title"]))
	}
	return out
}
