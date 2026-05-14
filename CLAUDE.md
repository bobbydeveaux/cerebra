# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

**Cerebra** — a Go CLI tool forked from [Fortress](~/code/newlife/fortress) that combines codebase knowledge indexing with persistent AI agent memory. It scans codebases, indexes AI agent conversations, generates embeddings, and stores them in a local SQLite vector database (Jor-El). Queryable via MCP server, CLI, and web UI.

Full documentation lives in `docs/`:
- [`docs/HLD.md`](docs/HLD.md) — system architecture, component overview, data flows, strategic vision
- [`docs/PRD.md`](docs/PRD.md) — original Fortress product requirements (inherited)
- [`docs/LLD.md`](docs/LLD.md) — original Fortress low-level design (inherited, will diverge)

## Origin

Cerebra is a standalone fork of Fortress. Fortress remains unchanged at `~/code/newlife/fortress` as a pure codebase/Confluence indexer. Cerebra inherits all Fortress capabilities and adds agent memory features on top.

## Commands

```bash
# Build
go build -o cerebra .

# Run all tests
go test ./...

# Scan a directory
./cerebra scan ./path/to/repos

# Start MCP server (for Claude Code)
./cerebra serve

# Start web UI (wiki + chat) on :8080
./cerebra serve --ui

# Watch filesystem and re-index on change
./cerebra watch
```

## Architecture in Brief

The inherited scan pipeline flows: **Scanner -> Chunker -> Embedder -> Store -> Doc Generator**

Three runtime modes share the same SQLite DB (`.cerebra/jor-el.db`):
1. `cerebra scan` — ingestion pipeline (codebases, Confluence)
2. `cerebra serve` — MCP stdio server (Claude Code integration)
3. `cerebra serve --ui` — HTTP server (wiki + chat web UI)

Embeddings default to **Ollama** (`nomic-embed-text`) locally; OpenAI is a configurable alternative. The vector DB uses **sqlite-vec** extension. Chat uses a RAG pipeline.

## Cerebra-Specific Additions (Planned)

What Cerebra adds beyond Fortress:

- **Conversation watcher** (`fsnotify`) — monitors `~/.claude/projects/*/` for JSONL session files
- **Conversation ingestion** — parses Claude Code JSONL into common schema, indexes via existing pipeline
- **Summary layer** — generates concise summaries per session to reduce token usage
- **Agent Brain Registry** — tracks all known agent sessions with metadata
- **Extended MCP tools** — `search_brain`, `list_brains`, `get_brain_summary`, `get_related_context`
- **Brain-aware UI** — dashboards, brain list, cross-agent links
- **Cross-agent discovery** — agents find relevant context from other agents' sessions
- **Agent Meeting Mode** — structured multi-agent discussions producing wiki artefacts

**Claude Code conversation files** are stored at `~/.claude/projects/<project-path>/<session-id>.jsonl` (JSONL, one JSON message per line, append-only during a session). Sub-agent logs live in `<session-id>/subagents/`. Session index at `~/.claude/sessions/<pid>.json`.

## Config

Default config file: `cerebra.yaml` in the working directory. DB default: `.cerebra/jor-el.db`.

## Key Design Decisions

- **Pure-Go SQLite driver** (`modernc.org/sqlite`) for cross-platform builds without CGO
- **htmx** for the web UI — no JavaScript build step
- **Incremental scans** track `last_commit_sha` per repo and `content_hash` per file
- **fsnotify** for filesystem watching — FSEvents (macOS), inotify (Linux), cross-platform
- **Summaries over raw context** — keep agent prompts concise
- **Agent-agnostic** — common schema normalises across Claude Code, Cursor, Copilot, etc.

## MVP Scope

1. Watch `~/.claude/projects/*/` for new/modified `*.jsonl` conversation files
2. Incrementally index conversation text
3. Generate/update one summary per session
4. Expose `search_brain` and `list_brains` via MCP
5. Basic UI table of known brains

**Success criteria:** A new Claude Code session can discover useful context from a previous session without manually copying prompts or summaries.
