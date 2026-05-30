# Changelog

## [Unreleased]

### Added
- **Test workflow with `sqlite_fts5` build tag** — GitHub Actions runs `go test ./...` with the build tag required for the FTS5 search path (#4)
- **`internal/web` test suite** — handler coverage raised from 7.7% to 83.9%, exercising wiki, search, and chat handlers (#5)
- **`internal/brain` test suite** — registry, session loading, and summary paths covered, 0% to 78.1% (#6)
- **`internal/scanner` test suite** — language detection, categorisation, and walk logic covered, 25% to 91% (#7)
- **`internal/store` test suite** — query helpers and write paths covered, 42% to 84.5% (#8)
- **`internal/embedder` test suite** — Ollama and OpenAI providers covered, 32.8% to 95.5% (#9)
- **`internal/rag` test suite** — pipeline assembly, prompt building, and streaming covered, 0% to 90.6% (#10)
- **`internal/datasource/confluence` test suite** — connector, pagination, and chunk emission covered, 0% to 95.3% (#11)
- **`internal/mcp` test suite** — tool dispatch and response shaping covered, 0% to 99.2% (#12)
- **`internal/storage` test suite** — GCS and S3 sync paths covered, 0% to 90% (#13)
- **`internal/chunker` test suite** — function, heading, and whole-file chunkers covered, 65.6% to 96.7% (#14)
- **`internal/config` test suite** — YAML loading, env-var override, and validation covered, 0% to 95.2% (#15)
- **`internal/store` agent_messages test suite** — read and write paths fully covered, 89.4% to 100% (#16)
- **`docs/LLD.md` Confluence + storage + coverage sections** — documents the Confluence datasource connector, GCS/S3 storage backends, and the 60% coverage bar (#17)
- **Structured HTTP request logging middleware** — `internal/web/logging.go` injects request ID, method, path, status, latency, and bytes written into every HTTP response (#18)
- **CHANGELOG `[Unreleased]` section populated** — sprint PRs #4 to #18 documented in the established format (#19)
- **`/health` endpoint** — `GET /health` returns `200 {"status":"ok","version":"dev"}` for Cloud Run liveness and readiness probes (#20)
- **`internal/docs` test suite** — generator, index, and per-category render paths covered, 0% to 96.6% (#21)
- **StackRamp `healthcheck_path: /health`** — Cloud Run liveness probe wired in `stackramp.yaml` so the platform polls the dedicated health endpoint instead of the default `/` (#23)
- **CHANGELOG `[Unreleased]` extended for PRs #19-#22** — sprint entries for the docs sweep, `/health`, `internal/docs` coverage, and the parser cognitive complexity refactor documented in-format (#24)
- **`internal/web` `/health` integration test** — locks the probe contract at byte level (`200 {"status":"ok","version":"dev"}`) and asserts `405 Method Not Allowed` for non-GET verbs (#25)
- **`internal/web/chat.go` test suite** — chat endpoint coverage raised from 21.6% to 96.1% via an injectable `chatPipeline` interface; all RAG paths exercised against a stub (#26)
- **`cmd/` test suite** — cobra command graph, flag defaults, arg validators, and runner happy/error paths covered, 0% to 64.3% (#27)

### Changed
- Internal package test coverage floor raised from 7.7% to 95%+ across all 12 internal packages plus the `agent_messages` subset of `internal/store` (#4-#16)
- **`internal/brain/parser.go` cognitive complexity** — `ParseSessionFile` split into focused helpers, complexity reduced from ~101 to under 70 to clear the StackRamp quality gate (#22)
- **`runWatch` and `runBrainsWatch` honour `cmd.Context()`** — watch handlers now propagate the cobra command context (falling back to `context.Background()` when called outside `ExecuteContext`) for clean ctrl-C shutdown and test cancellation (#27)

### Fixed
- **CI silently used a degraded FTS5 code path** — the `sqlite_fts5` build tag was not wired into the CI test step, so the FTS5-backed search code was untested in the pipeline (#4)

## [0.2.0] - 2026-04-18

### Added
- **Multiple chat LLM providers**: OpenAI (GPT-4o), MiniMax, Claude, and Ollama supported for RAG chat
- **Chat conversation history**: Follow-up questions now have context from previous messages
- **MCP search fallback**: MCP server search tool now falls back to FTS when vector search returns no results
- **Think tag stripping**: Filters out `<think>` reasoning blocks from LLM streaming output (MiniMax compatibility)

### Changed
- MCP search default limit increased from 5 to 10 results for richer context
- Improved RAG system prompt: more structured, concise answers with bullet points and citations
- Chat LLM default changed from Ollama to OpenAI for better response quality

### Fixed
- **Wiki page not rendering**: All page templates were parsed into one template set, causing `content` block collisions. Each page now gets its own isolated template set.
- **Template rendering**: Fixed `tmpl` to `tmpls` map-based template lookup across all handlers

## [0.1.0] - 2026-04-17

### Added
- Initial release of Fortress CLI tool
- Recursive codebase scanner with language detection and categorisation
- Intelligent chunking by function boundaries (code), headings (markdown), and whole-file (config)
- Embedding pipeline with Ollama (nomic-embed-text) and OpenAI support
- SQLite vector database with sqlite-vec extension for cosine similarity search
- FTS5 full-text search with porter stemming
- Incremental scanning via content hashes and git SHA tracking
- Git history indexing as searchable documents
- MCP server (stdio) with search, list_categories, get_document, get_stats tools
- CLI commands: scan, search, serve, stats, watch, forget
- Web UI with wiki (browse categories, files, chunks), search, and RAG chat
- Auto-generated markdown documentation (index.md + per-category docs)
- Cloud storage support (GCS, S3) for DB artifact sync
- Marketing site (Vite + React) with animated hero and full product narrative
- StackRamp deployment config for fortress.stackramp.io
- GitHub Actions workflow for automated deployment
- Configurable ignore patterns with ~/fortress.yaml fallback
- Colored CLI output with progress bars
