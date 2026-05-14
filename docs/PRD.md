# Fortress — Product Requirements Document (PRD)

**Version:** 1.0
**Status:** Draft
**Project:** Fortress
**Codename:** Jor-El

---

## 1. Vision

> *"On Krypton, Jor-El encoded all the world's knowledge into a crystal — so that his son, travelling through the void, would arrive on a new world already knowing everything."*

Codebases grow. Developers leave. Context gets lost. Onboarding new engineers (or AI assistants) to a large codebase means wading through thousands of files, stale wikis, and tribal knowledge that lives only in people's heads.

**Fortress** is a CLI tool and local knowledge platform that performs the Superman journey: it travels through every directory and repository, absorbs all knowledge, and crystallises it into a persistent, searchable, conversational knowledge base — the **Jor-El database**.

Fortress can be queried via:
- **CLI** (`fortress search "how does auth work"`)
- **MCP server** (Claude Code and other AI tools talk to Jor-El directly)
- **Web UI** — a local (or cloud-hosted) wiki with a chat interface

The result: any developer or AI assistant can ask questions about the codebase and get accurate, cited answers — instantly.

---

## 2. Problem Statement

| Problem | Impact |
|---------|--------|
| Large codebases span many repos, languages, and frameworks | No single place to understand the whole system |
| Documentation is scattered, stale, or nonexistent | New developers and AI tools lack context |
| Knowledge silos form as teams grow | Onboarding is slow and expensive |
| AI coding assistants have no project-wide context | They make incorrect assumptions about architecture |
| Manual wiki maintenance is tedious and always lags behind code | Documentation drifts from reality |

Fortress solves this by **automatically deriving knowledge from the codebase itself** and keeping it fresh — not by asking developers to write more docs.

---

## 3. Target Users

| User | Description |
|------|-------------|
| **Individual developer** | Wants a searchable, chat-able knowledge base for their own projects |
| **Engineering team** | Wants a shared knowledge base deployed to the cloud that everyone can query |
| **AI assistant (Claude Code etc.)** | Connects to Fortress via MCP and uses Jor-El as a persistent memory layer |
| **New joiners / onboardees** | Use the wiki + chat to understand the codebase without bothering senior engineers |
| **CI/CD pipelines** | Run `fortress scan --ci` on merge to keep the knowledge base up to date |

---

## 4. User Stories

### Core: Scanning & Indexing

- **US-01**: As a developer, I can run `fortress scan .` in any directory and have Fortress recursively discover and index all files, so I get a complete knowledge base without manual configuration.
- **US-02**: As a developer, I can re-run `fortress scan` and only changed files are re-processed, so incremental updates are fast and cheap.
- **US-03**: As a developer, I can run `fortress watch .` and have Fortress automatically re-index files as I edit them in real-time.
- **US-04**: As a CI/CD operator, I can run `fortress scan --ci --upload gcs://bucket/fortress.db` in a pipeline so the knowledge base is always current after every merge to main.
- **US-05**: As a developer, I can configure which files and directories to ignore (e.g. `node_modules`, `vendor`, binaries) via `fortress.yaml`.

### Core: Search & Query

- **US-06**: As a developer, I can run `fortress search "how does the payment flow work"` and get a ranked list of relevant code and documentation chunks with source file references.
- **US-07**: As an AI assistant (via MCP), I can call the `search` tool with a natural language query and receive semantically relevant results from the knowledge base.
- **US-08**: As a developer, I can run `fortress stats` to see a summary of what Jor-El knows — repos indexed, file counts, categories discovered, last scan time.

### Web UI: Wiki

- **US-09**: As a developer, I can run `fortress serve --ui` and browse an auto-generated wiki of my codebase in a local web browser.
- **US-10**: As a developer, I can navigate the wiki by category (e.g. "APIs", "Infrastructure", "Frontend"), by repository, and by file type.
- **US-11**: As a developer, I can search the wiki and see semantically relevant results highlighted in context.

### Web UI: Chat

- **US-12**: As a developer, I can chat with Jor-El via the web UI: ask questions in natural language and receive answers that cite specific source files.
- **US-13**: As a developer, Jor-El's chat responses include clickable links back to the relevant source files and wiki pages.
- **US-14**: As a developer, I can choose whether the chat LLM runs locally (via Ollama) or uses the Claude API, configured in `fortress.yaml`.

### Cloud & Teams

- **US-15**: As a team lead, I can configure Fortress to upload the knowledge base DB to GCS or S3, so all team members share the same Jor-El.
- **US-16**: As a developer on a team, I can run `fortress serve --db gcs://bucket/fortress.db` to pull the shared knowledge base and run the wiki/chat locally.
- **US-17**: As a team, the knowledge base stays fresh via a CI job that runs `fortress scan --ci` on every merge to main.

### MCP Integration

- **US-18**: As a Claude Code user, I can add Fortress as an MCP server in my settings so Claude can query the knowledge base in any conversation.
- **US-19**: As an AI assistant, I can use the `list_categories` MCP tool to discover what domains of knowledge Jor-El has indexed.
- **US-20**: As an AI assistant, I can use the `get_document` MCP tool to retrieve the full content of a specific indexed document.

---

## 5. Feature Requirements

### 5.1 Scanner

| ID | Requirement |
|----|-------------|
| F-01 | Recursively walk all directories from the given path |
| F-02 | Detect and index: source code (all languages), markdown/docs, config files (YAML/TOML/JSON/env), Makefiles/Dockerfiles/CI configs, git history (commit messages, authors, timestamps) |
| F-03 | Detect binary files and skip them automatically |
| F-04 | Support configurable ignore patterns (glob syntax) |
| F-05 | Detect git repositories and extract per-repo metadata (name, remote URL, last commit) |
| F-06 | Associate each file with the nearest git repository root |
| F-07 | Support `--dry-run` to preview what would be scanned |

### 5.2 Chunker

| ID | Requirement |
|----|-------------|
| F-08 | Split source code files by function/class/method boundaries |
| F-09 | Split markdown/documentation files by heading hierarchy |
| F-10 | Keep config files whole (they are typically small) |
| F-11 | Group git commit messages by time window or logical feature |
| F-12 | Each chunk carries metadata: source file path, repo, line range, file type, language, category |
| F-13 | Configurable chunk size (default 512 tokens) |

### 5.3 Embedder

| ID | Requirement |
|----|-------------|
| F-14 | Default to Ollama for local embedding generation (model: `nomic-embed-text`) |
| F-15 | Support OpenAI `text-embedding-3-small` as a configurable alternative |
| F-16 | Batch embed chunks concurrently for performance |
| F-17 | Show a progress bar during embedding (this is the slow step) |
| F-18 | Gracefully retry on transient errors (rate limits, timeouts) |

### 5.4 Vector Storage

| ID | Requirement |
|----|-------------|
| F-19 | Store all documents and embeddings in a local SQLite database using the sqlite-vec extension |
| F-20 | Support cosine similarity search over embeddings |
| F-21 | Support full-text search (FTS5) as a fallback and for hybrid search |
| F-22 | Store per-file content hashes and per-repo last-scanned commit SHA for incremental sync |
| F-23 | Database lives at `.fortress/jor-el.db` by default (configurable) |

### 5.5 Incremental Sync

| ID | Requirement |
|----|-------------|
| F-24 | On re-scan, detect changed files via git diff (for git repos) or content hash comparison (for non-git dirs) |
| F-25 | Only re-chunk and re-embed changed or new files |
| F-26 | Remove documents from the DB for files that have been deleted |
| F-27 | `fortress watch` uses filesystem events to re-index files as they change |
| F-28 | `fortress scan --full` forces a complete re-scan, ignoring incremental state |

### 5.6 Document Generator

| ID | Requirement |
|----|-------------|
| F-29 | After each scan, generate a `docs/index.md` master index document |
| F-30 | Generate per-category sub-documents (e.g. `docs/categories/api.md`) |
| F-31 | Categories are auto-detected from directory structure, file types, and content patterns |
| F-32 | Generated docs include: category summary, list of key files, notable patterns observed |
| F-33 | Generated docs are always re-generated to stay fresh |

### 5.7 MCP Server

| ID | Requirement |
|----|-------------|
| F-34 | Expose a Model Context Protocol server via stdio transport |
| F-35 | Implement `search` tool: accepts a natural language query, returns top-k semantically relevant chunks |
| F-36 | Implement `list_categories` tool: returns all discovered categories with descriptions |
| F-37 | Implement `get_document` tool: returns full content of a specific indexed document |
| F-38 | Implement `get_stats` tool: returns summary statistics about the knowledge base |
| F-39 | MCP server is started with `fortress serve` (no flags) |

### 5.8 Web UI — Wiki

| ID | Requirement |
|----|-------------|
| F-40 | Serve a web UI on `localhost:8080` (configurable) via `fortress serve --ui` |
| F-41 | Wiki navigation: browse by category, by repository, by file type |
| F-42 | Each wiki page renders the generated markdown documentation with syntax-highlighted code |
| F-43 | Search bar performs semantic + full-text hybrid search |
| F-44 | Search results link back to source files |

### 5.9 Web UI — Chat

| ID | Requirement |
|----|-------------|
| F-45 | Chat panel in the web UI for natural language Q&A |
| F-46 | RAG pipeline: query → embed → vector search → top-k chunks → LLM prompt → streamed response |
| F-47 | Chat responses include citations: source file paths, line ranges, links to wiki pages |
| F-48 | Configurable LLM backend: Ollama (local) or Claude API |
| F-49 | Chat is streamed (server-sent events) for responsiveness |

### 5.10 Cloud Storage

| ID | Requirement |
|----|-------------|
| F-50 | `fortress scan --upload <uri>` uploads the DB to GCS or S3 after scanning |
| F-51 | `fortress serve --db <uri>` downloads the DB from GCS or S3 on startup |
| F-52 | Support `gcs://bucket/path` and `s3://bucket/path` URI schemes |
| F-53 | Cloud credentials sourced from standard environment (Application Default Credentials for GCS, AWS SDK chain for S3) |

---

## 6. Non-Functional Requirements

| ID | Requirement |
|----|-------------|
| NF-01 | **Cross-platform**: Runs on macOS, Linux, Windows without modification. Single binary distribution. |
| NF-02 | **Offline-first**: All core functionality works without internet access (using Ollama for embeddings and LLM) |
| NF-03 | **Privacy**: No data leaves the machine unless cloud storage is explicitly configured |
| NF-04 | **Performance**: Incremental scans complete in seconds for typical codebases (< 10k changed files) |
| NF-05 | **Concurrency**: Embedding generation uses worker pools to saturate available CPU/GPU |
| NF-06 | **No external processes**: The DB requires no running server (SQLite is embedded) |
| NF-07 | **Single binary**: `go build` produces one self-contained executable with no runtime dependencies |
| NF-08 | **Configurable**: All defaults can be overridden via `fortress.yaml` or CLI flags |

---

## 7. CLI Reference

```
fortress scan [path]              Scan, chunk, embed, store, generate docs
fortress scan --dry-run           Preview what would be scanned
fortress scan --full              Force full re-scan (ignore incremental state)
fortress scan --ci --upload URI   CI mode: scan then push DB to cloud

fortress watch [path]             Watch filesystem, re-index on change

fortress serve                    Start MCP server (stdio, for Claude Code)
fortress serve --ui               Start web UI (wiki + chat) on :8080
fortress serve --ui --port 9000   Custom port
fortress serve --db gcs://...     Serve from cloud-stored DB

fortress search "query"           Semantic CLI search
fortress stats                    Show knowledge base summary
fortress forget [path]            Remove a path from the knowledge base
```

---

## 8. Out of Scope (v1)

- **Multi-user auth** on the web UI — it's a local tool; cloud deployments are trusted-network only
- **Real-time collaboration** — the DB is a single-writer file
- **Automatic language-specific AST parsing** — chunking uses regex-based heuristics in v1; tree-sitter integration is a v2 enhancement
- **Support for binary assets** (images, compiled artifacts, databases)
- **Turso/libSQL cloud replication** — GCS/S3 file sync is sufficient for v1
- **Plugin system** for custom file type handlers
- **Web-based scan triggering** — scans are always initiated from the CLI

---

## 9. Success Metrics

| Metric | Target |
|--------|--------|
| Time to first scan (typical repo, ~5k files) | < 5 minutes |
| Incremental rescan time (< 100 changed files) | < 30 seconds |
| Search relevance (manual evaluation) | Top result is relevant > 80% of the time |
| MCP query latency | < 2 seconds |
| Web UI page load time | < 500ms (local) |
