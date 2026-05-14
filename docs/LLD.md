# Fortress — Low-Level Design (LLD)

**Version:** 1.0
**Status:** Draft

---

## 1. Directory & Package Structure

```
fortress/
├── main.go                         Entry point; calls cmd.Execute()
├── go.mod
├── go.sum
├── fortress.yaml                   Default config (committed to repo)
│
├── cmd/                            Cobra CLI commands
│   ├── root.go                     Root command; loads config
│   ├── scan.go                     `fortress scan`
│   ├── serve.go                    `fortress serve` (MCP + web)
│   ├── search.go                   `fortress search`
│   ├── watch.go                    `fortress watch`
│   ├── stats.go                    `fortress stats`
│   └── forget.go                   `fortress forget`
│
├── internal/
│   ├── config/
│   │   └── config.go               Config struct, load/validate from YAML + flags
│   │
│   ├── scanner/
│   │   ├── scanner.go              Filesystem walker; emits Documents
│   │   ├── gitlog.go               Git metadata extraction
│   │   ├── filetype.go             File type / language detection
│   │   └── incremental.go          Hash & git-diff based change detection
│   │
│   ├── chunker/
│   │   ├── chunker.go              Chunker interface + dispatcher
│   │   ├── code.go                 Source code chunking (function boundaries)
│   │   ├── markdown.go             Markdown chunking (by headings)
│   │   ├── config.go               Config file chunking (keep whole)
│   │   └── githistory.go           Git commit message chunking
│   │
│   ├── embedder/
│   │   ├── embedder.go             Embedder interface
│   │   ├── ollama.go               Ollama implementation
│   │   ├── openai.go               OpenAI implementation
│   │   └── pool.go                 Worker pool for concurrent embedding
│   │
│   ├── store/
│   │   ├── store.go                Store interface + SQLite implementation
│   │   ├── schema.go               SQL DDL (tables, indexes, virtual tables)
│   │   └── query.go                Search queries (vector + FTS5 hybrid)
│   │
│   ├── docs/
│   │   └── generator.go            Markdown doc generation from indexed content
│   │
│   ├── mcp/
│   │   └── server.go               MCP stdio server + tool handlers
│   │
│   ├── web/
│   │   ├── server.go               HTTP server, route registration
│   │   ├── wiki.go                 Wiki route handlers
│   │   ├── chat.go                 Chat + SSE stream handler
│   │   ├── templates/
│   │   │   ├── layout.html
│   │   │   ├── index.html
│   │   │   ├── category.html
│   │   │   ├── document.html
│   │   │   ├── search.html
│   │   │   └── chat.html
│   │   └── static/
│   │       ├── style.css
│   │       └── htmx.min.js         vendored htmx
│   │
│   ├── rag/
│   │   └── pipeline.go             RAG pipeline: query → embed → search → LLM → stream
│   │
│   └── storage/
│       ├── storage.go              Storage interface (upload/download DB file)
│       ├── local.go                LocalStorage (noop — reads from disk)
│       ├── gcs.go                  GCSStorage
│       └── s3.go                   S3Storage
```

---

## 2. Core Types

### 2.1 Domain Structs

```go
// internal/scanner/scanner.go

type Document struct {
    ID          string            // sha256 of path (stable ID)
    Path        string            // absolute file path
    RelPath     string            // path relative to scan root
    Repo        string            // git repo name (dir name or remote slug)
    RepoRoot    string            // absolute path to git repo root
    Category    Category          // detected category
    Language    string            // programming language, e.g. "go", "python"
    FileType    FileType          // code | markdown | config | data | git
    Content     string            // full raw content
    ContentHash string            // sha256 of Content
    ModTime     time.Time
    Metadata    map[string]string // arbitrary key-value (e.g. git remote URL)
}

type Category string
const (
    CategoryAPI        Category = "api"
    CategoryInfra      Category = "infrastructure"
    CategoryFrontend   Category = "frontend"
    CategoryTesting    Category = "testing"
    CategoryDocs       Category = "documentation"
    CategoryConfig     Category = "configuration"
    CategoryMigrations Category = "migrations"
    CategoryUnknown    Category = "unknown"
)

type FileType string
const (
    FileTypeCode       FileType = "code"
    FileTypeMarkdown   FileType = "markdown"
    FileTypeConfig     FileType = "config"
    FileTypeGitHistory FileType = "git_history"
    FileTypeData       FileType = "data"
)
```

```go
// internal/chunker/chunker.go

type Chunk struct {
    ID         string    // sha256 of (DocumentID + StartLine)
    DocumentID string
    Content    string
    StartLine  int
    EndLine    int
    Embedding  []float32 // populated by Embedder; nil before embedding
    Metadata   ChunkMeta
}

type ChunkMeta struct {
    Path     string
    Repo     string
    Category Category
    Language string
    FileType FileType
}
```

```go
// internal/store/store.go

type ScanState struct {
    Repo          string
    LastCommitSHA string
    LastScanTime  time.Time
    FileCount     int
    ChunkCount    int
}

type SearchResult struct {
    Chunk     Chunk
    Score     float32 // cosine similarity or hybrid score
    Highlight string  // snippet with query terms highlighted
}
```

### 2.2 Interfaces

```go
// internal/scanner/scanner.go
type Scanner interface {
    Scan(ctx context.Context, root string) (<-chan Document, <-chan error)
}

// internal/chunker/chunker.go
type Chunker interface {
    Chunk(doc Document) ([]Chunk, error)
}

// internal/embedder/embedder.go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dimensions() int
}

// internal/store/store.go
type Store interface {
    // Write
    UpsertDocument(ctx context.Context, doc Document, chunks []Chunk) error
    DeleteDocument(ctx context.Context, docID string) error
    UpdateScanState(ctx context.Context, state ScanState) error

    // Read
    Search(ctx context.Context, queryVec []float32, limit int) ([]SearchResult, error)
    SearchFTS(ctx context.Context, query string, limit int) ([]SearchResult, error)
    GetDocument(ctx context.Context, path string) (*Document, []Chunk, error)
    GetScanState(ctx context.Context, repo string) (*ScanState, error)
    GetStats(ctx context.Context) (Stats, error)
    ListCategories(ctx context.Context) ([]CategorySummary, error)

    Close() error
}

// internal/storage/storage.go
type Storage interface {
    Upload(ctx context.Context, localPath string) error
    Download(ctx context.Context, localPath string) error
}
```

---

## 3. SQLite Schema

```sql
-- internal/store/schema.go (executed on DB open)

PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- Repositories discovered during scan
CREATE TABLE IF NOT EXISTS repos (
    id          TEXT PRIMARY KEY,   -- slug (repo dir name)
    root_path   TEXT NOT NULL,
    remote_url  TEXT,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Source documents (one per file, plus virtual git history docs)
CREATE TABLE IF NOT EXISTS documents (
    id           TEXT PRIMARY KEY,  -- sha256(path)
    path         TEXT NOT NULL UNIQUE,
    rel_path     TEXT NOT NULL,
    repo_id      TEXT REFERENCES repos(id),
    category     TEXT NOT NULL,
    language     TEXT NOT NULL DEFAULT '',
    file_type    TEXT NOT NULL,
    content      TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    metadata     TEXT NOT NULL DEFAULT '{}',  -- JSON
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Chunks derived from documents
CREATE TABLE IF NOT EXISTS chunks (
    id          TEXT PRIMARY KEY,   -- sha256(doc_id + start_line)
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    content     TEXT NOT NULL,
    start_line  INT NOT NULL DEFAULT 0,
    end_line    INT NOT NULL DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- sqlite-vec virtual table for embeddings
-- Dimensions must match the configured embedding model
-- nomic-embed-text = 768, text-embedding-3-small = 1536
CREATE VIRTUAL TABLE IF NOT EXISTS chunk_embeddings USING vec0(
    chunk_id TEXT PRIMARY KEY,
    embedding FLOAT[768]            -- adjusted at init time from config
);

-- FTS5 full-text search index over chunks
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    content,
    document_id UNINDEXED,
    chunk_id UNINDEXED,
    tokenize = "porter ascii"
);

-- Tracks scan state per repo for incremental rescans
CREATE TABLE IF NOT EXISTS scan_state (
    repo_id        TEXT PRIMARY KEY REFERENCES repos(id),
    last_commit_sha TEXT NOT NULL DEFAULT '',
    last_scan_time  DATETIME NOT NULL,
    file_count      INT NOT NULL DEFAULT 0,
    chunk_count     INT NOT NULL DEFAULT 0
);

-- Auto-detected categories with descriptions
CREATE TABLE IF NOT EXISTS categories (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    file_count  INT NOT NULL DEFAULT 0,
    chunk_count INT NOT NULL DEFAULT 0,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

## 4. Chunking Algorithms

### 4.1 Source Code (`internal/chunker/code.go`)

Split by top-level function/class/method definitions using language-specific regex patterns.

```go
var codePatterns = map[string]*regexp.Regexp{
    "go":         regexp.MustCompile(`(?m)^func\s+`),
    "python":     regexp.MustCompile(`(?m)^(def |class )\s+`),
    "javascript": regexp.MustCompile(`(?m)^(function |const \w+ = |class )\s*`),
    "typescript": regexp.MustCompile(`(?m)^(function |const \w+ = |class |interface |type )\s*`),
    "rust":       regexp.MustCompile(`(?m)^(fn |impl |struct |enum |trait )\s+`),
    "java":       regexp.MustCompile(`(?m)^(\s+)(public|private|protected|static).*\{`),
    // fallback: split every N lines
}
```

**Algorithm:**
1. Find all pattern match positions in the file
2. Each match starts a new chunk
3. If a resulting chunk exceeds `max_tokens`, split further at blank lines
4. Minimum chunk size: 3 lines (discard tiny stubs)

### 4.2 Markdown (`internal/chunker/markdown.go`)

Split at heading boundaries (H1, H2, H3). Each section becomes a chunk including its heading.

```
# Top Level Heading         → start of chunk 1
  content...
## Sub Heading              → start of chunk 2
  content...
### Sub-sub Heading         → start of chunk 3
```

If a section exceeds `max_tokens`, split further at paragraph breaks (double newline).

### 4.3 Config Files (`internal/chunker/config.go`)

Keep the entire file as a single chunk. Config files (YAML, TOML, JSON, `.env`) are typically small and semantically unified — splitting them loses context.

Max size guard: if the file exceeds 10,000 characters, split at top-level keys.

### 4.4 Git History (`internal/chunker/githistory.go`)

Group commits into windows:
- Default window: 20 commits per chunk
- Format per chunk:
  ```
  [REPO: my-service] Git history 2024-01-15 to 2024-02-01

  abc1234 2024-02-01 Alice: Add payment retry logic
  def5678 2024-01-28 Bob: Fix race condition in worker pool
  ...
  ```

---

## 5. Embedding Pipeline

### 5.1 Worker Pool (`internal/embedder/pool.go`)

```
Chunks channel (buffered, size 1000)
       │
       ├─▶ Worker 1 ─▶ Embedder.Embed(batch) ─▶ Results channel
       ├─▶ Worker 2 ─▶ Embedder.Embed(batch) ─▶ Results channel
       └─▶ Worker N ─▶ Embedder.Embed(batch) ─▶ Results channel
```

- Default workers: `min(runtime.NumCPU(), 4)` (limited by Ollama's concurrency)
- Batch size: 32 chunks per API call (configurable)
- Each worker collects a batch, calls `Embedder.Embed`, emits enriched chunks

### 5.2 Ollama Embedder (`internal/embedder/ollama.go`)

```go
type OllamaEmbedder struct {
    BaseURL string // default: http://localhost:11434
    Model   string // default: nomic-embed-text
    client  *http.Client
}

// POST /api/embeddings
// Body: {"model": "nomic-embed-text", "prompt": "<text>"}
// Ollama does not support batch embedding natively — one request per chunk
// Use workers to parallelize
```

### 5.3 OpenAI Embedder (`internal/embedder/openai.go`)

```go
type OpenAIEmbedder struct {
    APIKey string
    Model  string // default: text-embedding-3-small
    client *http.Client
}

// POST https://api.openai.com/v1/embeddings
// Body: {"model": "text-embedding-3-small", "input": ["text1", "text2", ...]}
// Supports batching up to 2048 inputs per request
```

### 5.4 Retry Logic

Both embedders wrap calls with exponential backoff:
- Max retries: 3
- Initial delay: 500ms
- Backoff factor: 2x
- Retried errors: HTTP 429, 503, connection timeout

---

## 6. Incremental Sync Algorithm

### 6.1 Git Repositories (`internal/scanner/incremental.go`)

```
On fortress scan (for a git repo):

1. Load scan_state for this repo (last_commit_sha)
2. If last_commit_sha is empty → full scan (first time)
3. Else:
   a. Run: git -C <repo_root> diff --name-only <last_commit_sha>..HEAD
   b. Run: git -C <repo_root> diff --name-only --diff-filter=D <last_commit_sha>..HEAD (deleted files)
   c. changed_files = added + modified files from diff
   d. deleted_files = deleted files from diff
4. Scan only changed_files → chunk → embed → upsert
5. For each deleted_file: store.DeleteDocument(path)
6. Update scan_state with current HEAD sha
```

### 6.2 Non-Git Directories

```
On fortress scan (for a non-git dir):

1. Walk all files, compute sha256(content) for each
2. Compare against documents.content_hash in DB
3. changed = files where hash differs or not in DB
4. deleted = files in DB not found on filesystem
5. Process changed_files, delete deleted_files
```

### 6.3 Watch Mode (`cmd/watch.go`)

```go
watcher, _ := fsnotify.NewWatcher()
watcher.Add(rootPath) // recursive

for event := range watcher.Events {
    switch event.Op {
    case fsnotify.Write, fsnotify.Create:
        // debounce 500ms, then re-scan single file
        debouncer.Trigger(event.Name, func() {
            doc := scanner.ScanFile(event.Name)
            chunks := chunker.Chunk(doc)
            chunks = embedder.EmbedChunks(ctx, chunks)
            store.UpsertDocument(ctx, doc, chunks)
        })
    case fsnotify.Remove, fsnotify.Rename:
        store.DeleteDocument(ctx, event.Name)
    }
}
```

---

## 7. MCP Server Implementation

### 7.1 Protocol (`internal/mcp/server.go`)

Implements MCP spec over stdio (JSON-RPC 2.0 framing). Uses a simple read-loop:

```go
func (s *Server) Serve(ctx context.Context) error {
    scanner := bufio.NewScanner(os.Stdin)
    enc := json.NewEncoder(os.Stdout)
    for scanner.Scan() {
        var req MCPRequest
        json.Unmarshal(scanner.Bytes(), &req)
        resp := s.handle(ctx, req)
        enc.Encode(resp)
    }
    return nil
}
```

### 7.2 Tool Definitions

```json
{
  "tools": [
    {
      "name": "search",
      "description": "Semantic search across the indexed codebase knowledge base",
      "inputSchema": {
        "type": "object",
        "properties": {
          "query": {"type": "string", "description": "Natural language search query"},
          "limit": {"type": "integer", "default": 5, "description": "Number of results"}
        },
        "required": ["query"]
      }
    },
    {
      "name": "list_categories",
      "description": "List all categories discovered in the indexed codebase",
      "inputSchema": {"type": "object", "properties": {}}
    },
    {
      "name": "get_document",
      "description": "Retrieve a specific indexed document by file path",
      "inputSchema": {
        "type": "object",
        "properties": {
          "path": {"type": "string", "description": "Relative file path"}
        },
        "required": ["path"]
      }
    },
    {
      "name": "get_stats",
      "description": "Get statistics about what Jor-El knows",
      "inputSchema": {"type": "object", "properties": {}}
    }
  ]
}
```

---

## 8. Web UI Routes & Templates

### 8.1 Routes

```go
// internal/web/server.go
mux.HandleFunc("GET /", handleIndex)                   // category list
mux.HandleFunc("GET /categories/{name}", handleCategory)
mux.HandleFunc("GET /repos/{name}", handleRepo)
mux.HandleFunc("GET /files/{path...}", handleFile)
mux.HandleFunc("GET /search", handleSearch)
mux.HandleFunc("GET /chat", handleChatPage)
mux.HandleFunc("POST /api/search", handleSearchAPI)    // htmx fragment
mux.HandleFunc("GET /api/chat/stream", handleChatStream) // SSE
mux.Handle("GET /static/", http.FileServer(staticFS))
```

### 8.2 Chat SSE Stream (`internal/web/chat.go`)

```go
func handleChatStream(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")

    query := r.URL.Query().Get("q")

    // 1. Embed query
    // 2. Vector search → top-k chunks
    // 3. Build prompt
    // 4. Stream from LLM → write SSE events
    for token := range llm.Stream(ctx, prompt) {
        fmt.Fprintf(w, "data: %s\n\n", token)
        flusher.Flush()
    }
    fmt.Fprintf(w, "data: [DONE]\n\n")
}
```

---

## 9. RAG Pipeline (`internal/rag/pipeline.go`)

```go
type Pipeline struct {
    embedder embedder.Embedder
    store    store.Store
    llm      LLMClient
}

func (p *Pipeline) Answer(ctx context.Context, question string) (<-chan string, error) {
    // Step 1: embed the question
    vecs, _ := p.embedder.Embed(ctx, []string{question})

    // Step 2: retrieve top-k chunks
    results, _ := p.store.Search(ctx, vecs[0], 8)

    // Step 3: build prompt
    var ctx strings.Builder
    for i, r := range results {
        fmt.Fprintf(&ctx, "[%d] File: %s (lines %d-%d)\n%s\n\n",
            i+1, r.Chunk.Metadata.Path, r.Chunk.StartLine, r.Chunk.EndLine, r.Chunk.Content)
    }
    prompt := buildPrompt(question, ctx.String())

    // Step 4: stream from LLM
    return p.llm.Stream(ctx, prompt)
}

func buildPrompt(question, context string) string {
    return fmt.Sprintf(`You are Jor-El, an expert on this codebase.
Answer the developer's question using only the provided context.
Cite sources using [N] notation referring to the context blocks above.
If the answer is not in the context, say so.

Context:
%s

Question: %s

Answer:`, context, question)
}
```

### LLM Client Interface

```go
type LLMClient interface {
    Stream(ctx context.Context, prompt string) (<-chan string, error)
}

// OllamaLLM: POST /api/generate with stream:true
// ClaudeLLM: Anthropic SDK streaming messages API
```

---

## 10. Configuration Schema

**`fortress.yaml`** (full annotated schema):

```yaml
# Embedding model provider: "ollama" or "openai"
embedder: ollama

# Ollama settings (used when embedder: ollama)
ollama:
  url: http://localhost:11434
  embed_model: nomic-embed-text    # 768 dimensions
  chat_model: llama3.2             # used for web UI chat

# OpenAI settings (used when embedder: openai)
openai:
  api_key: ""                      # prefer env: OPENAI_API_KEY
  embed_model: text-embedding-3-small

# Chat LLM: "ollama" or "claude"
chat_llm: ollama

# Claude API settings (used when chat_llm: claude)
claude:
  api_key: ""                      # prefer env: ANTHROPIC_API_KEY
  model: claude-sonnet-4-6

# Files/dirs to ignore during scan (glob patterns)
ignore:
  - .git
  - node_modules
  - vendor
  - .venv
  - __pycache__
  - "*.bin"
  - "*.exe"
  - "*.so"
  - "*.dylib"
  - dist
  - build
  - coverage

# Chunking
chunk_size: 512                    # approximate token count per chunk
chunk_overlap: 64                  # tokens of overlap between adjacent chunks

# Storage
db_path: .fortress/jor-el.db      # local DB path
docs_path: .fortress/docs/        # generated docs path

# Cloud storage URI (optional)
# Formats: gcs://bucket/path  or  s3://bucket/path
cloud_storage: ""

# Web UI
ui_port: 8080
ui_bind: 127.0.0.1                 # change to 0.0.0.0 for network access

# Embedding worker concurrency
embed_workers: 4
embed_batch_size: 32
```

---

## 11. Key Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `github.com/spf13/cobra` | v1.8+ | CLI framework |
| `github.com/spf13/viper` | v1.18+ | Config loading (YAML + env + flags) |
| `modernc.org/sqlite` | v1.29+ | Pure-Go SQLite driver (no CGO, easy cross-compile) |
| `github.com/asg017/sqlite-vec-go-bindings` | latest | sqlite-vec extension for vector search |
| `gopkg.in/yaml.v3` | v3 | YAML config parsing |
| `github.com/schollz/progressbar/v3` | v3 | Progress bar during embedding |
| `github.com/fsnotify/fsnotify` | v1.7+ | Filesystem event watching |
| `cloud.google.com/go/storage` | v1.40+ | GCS client |
| `github.com/aws/aws-sdk-go-v2/service/s3` | v1+ | S3 client |

> **Note on sqlite-vec:** The `modernc.org/sqlite` pure-Go driver and sqlite-vec may require CGO for the vec extension. Evaluate `github.com/mattn/go-sqlite3` (CGO) as an alternative if the pure-Go path proves incompatible with sqlite-vec. CGO complicates cross-compilation but is a known-working path.

---

## 12. Error Handling Patterns

- **Scanner errors**: Log and continue (one bad file should not abort the scan); collect errors and report summary at end
- **Embedder errors**: Retry with backoff (see §5.4); if all retries fail, skip chunk and log warning
- **Store errors**: Fatal — DB corruption or full disk should abort the scan
- **MCP errors**: Return JSON-RPC error response; never panic
- **Web errors**: Return HTTP 500 with error detail in development; generic message in production

---

## 13. Testing Strategy

### Unit Tests
- `internal/chunker/*_test.go` — test each chunking strategy against fixture files
- `internal/embedder/pool_test.go` — test worker pool with a mock embedder
- `internal/store/store_test.go` — test DB operations against an in-memory SQLite DB
- `internal/scanner/incremental_test.go` — test change detection logic with mock git output

### Integration Tests
- `integration/scan_test.go` — run a full scan against a small fixture repository in `testdata/`
- `integration/search_test.go` — index fixture data then verify search returns expected results
- `integration/mcp_test.go` — start MCP server, send JSON-RPC requests over stdio, assert responses

### Test Fixtures
- `testdata/repo/` — a small fake repository with multiple file types
- `testdata/git/` — pre-seeded git log for testing git history extraction

### Running Tests
```bash
go test ./...                      # all unit tests
go test ./integration/...          # integration tests (requires Ollama running)
go test -run TestChunker ./internal/chunker/...   # single test
```
