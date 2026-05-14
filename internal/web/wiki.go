package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bobbydeveaux/cerebra/internal/store"
)

type DirEntry struct {
	Name     string
	Path     string
	IsDir    bool
	Language string
	Children []*DirEntry
}

type DirGroup struct {
	Dir   string
	Files []store.FileSummary
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	ctx := context.Background()

	stats, _ := s.store.GetStats(ctx)
	categories, _ := s.store.ListCategories(ctx)

	files, _ := s.store.ListAllFiles(ctx)
	topDirs := buildTopDirs(files)

	data := map[string]interface{}{
		"Title":      "Jor-El Knowledge Base",
		"Stats":      stats,
		"Categories": categories,
		"TopDirs":    topDirs,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpls["index.html"].ExecuteTemplate(w, "index.html", data); err != nil {
		fmt.Fprintf(os.Stderr, "index template error: %v\n", err)
		http.Error(w, "Template error: "+err.Error(), 500)
	}
}

func (s *Server) handleCategory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := context.Background()

	categories, _ := s.store.ListCategories(ctx)

	var cat *store.CategorySummary
	for _, c := range categories {
		if c.Name == name {
			cat = &c
			break
		}
	}

	if cat == nil {
		http.NotFound(w, r)
		return
	}

	files, _ := s.store.ListFilesByCategory(ctx, name, 500)

	// Group by directory
	dirs := make(map[string][]store.FileSummary)
	for _, f := range files {
		dir := filepath.Dir(f.RelPath)
		dirs[dir] = append(dirs[dir], f)
	}
	dirNames := make([]string, 0, len(dirs))
	for d := range dirs {
		dirNames = append(dirNames, d)
	}
	sort.Strings(dirNames)

	var groups []DirGroup
	for _, d := range dirNames {
		groups = append(groups, DirGroup{Dir: d, Files: dirs[d]})
	}

	data := map[string]interface{}{
		"Title":    "Category: " + name,
		"Category": cat,
		"Groups":   groups,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.tmpls["category.html"].ExecuteTemplate(w, "category.html", data)
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	ctx := context.Background()

	doc, chunks, err := s.store.GetDocument(ctx, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	data := map[string]interface{}{
		"Title":    filepath.Base(path),
		"Document": doc,
		"Chunks":   chunks,
		"Path":     path,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.tmpls["document.html"].ExecuteTemplate(w, "document.html", data)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Title": "Search",
		"Query": r.URL.Query().Get("q"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.tmpls["search.html"].ExecuteTemplate(w, "search.html", data)
}

func (s *Server) handleSearchAPI(w http.ResponseWriter, r *http.Request) {
	query := r.FormValue("q")
	if query == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Vector search (same as CLI)
	var results []store.SearchResult
	vecs, err := s.embedder.Embed(ctx, []string{query})
	if err == nil {
		results, _ = s.store.Search(ctx, vecs[0], 10)
	}

	// Fallback to FTS if vector search returned nothing
	if len(results) == 0 {
		results, _ = s.store.SearchFTS(ctx, query, 10)
	}

	if len(results) == 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<div class="no-results">No results found.</div>`))
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	for _, r := range results {
		content := r.Chunk.Content
		if len(content) > 300 {
			content = content[:300] + "..."
		}

		score := fmt.Sprintf("%.2f", r.Score)
		w.Write([]byte(`<div class="result">`))
		w.Write([]byte(`<div class="result-header">`))
		w.Write([]byte(`<a href="/files/` + r.Chunk.Metadata.Path + `" class="result-file">` + r.Chunk.Metadata.Path + `</a>`))
		if r.Chunk.StartLine > 0 {
			w.Write([]byte(`<span class="result-lines">:` + itoa(r.Chunk.StartLine) + `-` + itoa(r.Chunk.EndLine) + `</span>`))
		}
		w.Write([]byte(`<span class="result-score">` + score + `</span>`))
		w.Write([]byte(`</div>`))
		if r.Chunk.Metadata.Repo != "" {
			w.Write([]byte(`<div class="result-meta">` + r.Chunk.Metadata.Repo + ` / ` + string(r.Chunk.Metadata.Category) + `</div>`))
		}
		w.Write([]byte(`<pre class="result-content">` + template_escape(content) + `</pre>`))
		w.Write([]byte(`</div>`))
	}
}

type TopDir struct {
	Name      string
	FileCount int
}

func buildTopDirs(files []store.FileSummary) []TopDir {
	counts := make(map[string]int)
	for _, f := range files {
		parts := strings.SplitN(f.RelPath, string(filepath.Separator), 2)
		dir := parts[0]
		if len(parts) == 1 {
			dir = "."
		}
		counts[dir]++
	}

	var dirs []TopDir
	for name, count := range counts {
		dirs = append(dirs, TopDir{Name: name, FileCount: count})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].FileCount > dirs[j].FileCount
	})
	return dirs
}

func buildDirTree(files []store.FileSummary) []*DirEntry {
	root := &DirEntry{Name: "", IsDir: true}

	for _, f := range files {
		parts := strings.Split(f.RelPath, string(filepath.Separator))
		current := root
		for i, part := range parts {
			isLast := i == len(parts)-1
			var child *DirEntry
			for _, c := range current.Children {
				if c.Name == part {
					child = c
					break
				}
			}
			if child == nil {
				child = &DirEntry{
					Name:  part,
					Path:  strings.Join(parts[:i+1], "/"),
					IsDir: !isLast,
				}
				if isLast {
					child.Language = f.Language
				}
				current.Children = append(current.Children, child)
			}
			current = child
		}
	}

	return root.Children
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func template_escape(s string) string {
	var result []byte
	for _, c := range s {
		switch c {
		case '<':
			result = append(result, []byte("&lt;")...)
		case '>':
			result = append(result, []byte("&gt;")...)
		case '&':
			result = append(result, []byte("&amp;")...)
		case '"':
			result = append(result, []byte("&quot;")...)
		default:
			result = append(result, byte(c))
		}
	}
	return string(result)
}
