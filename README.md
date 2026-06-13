# Cerebra

**Persistent memory for AI agents.**

Cerebra watches your AI agent conversations in real time, indexes every interaction into a local vector database, generates concise summaries, and makes it all searchable — so every agent session starts with the full picture instead of from zero.

Built on top of [Fortress](https://github.com/bobbydeveaux/fortress) (codebase indexing), Cerebra extends the pipeline with agent-aware ingestion, brain tracking, and cross-agent discovery.

**Name origin:** Cerebra is the plural of cerebrum — multiple brains, unified.

[Website](https://cerebra.stackramp.io) &bull; [Docs](https://cerebra.stackramp.io/docs) &bull; [GitHub](https://github.com/bobbydeveaux/cerebra)

---

## The Problem

AI coding agents — Claude Code, Copilot, Cursor — have no memory between sessions. Every conversation, every decision, every hard-won insight vanishes the moment the session ends. The next agent starts completely blind.

Running multiple agents across repos? They can't share what they've learned. Agent A refactors auth while Agent B re-implements the old pattern in another service.

Without persistent memory, your AI agents don't get smarter over time.

## How Cerebra Solves It

```
Agent Session (JSONL)
       ↓
Filesystem Watcher (fsnotify)
       ↓
Parser → Common Schema
       ↓
Chunker → Embedder (Ollama / OpenAI)
       ↓
SQLite Vector DB (Jor-El)
       ↓
MCP Server / Web UI / Other Agents
```

1. **Watch** — Monitors `~/.claude/projects/` for JSONL conversation files using native filesystem events. New content detected within seconds.
2. **Index** — Parses conversations into a common schema, chunks semantically, embeds into a local SQLite vector database. Incremental by default.
3. **Summarise** — Each session gets a continuously-updated summary. Key decisions, patterns, insights distilled into compact context.
4. **Connect** — Exposes the full knowledge base via MCP. Any agent can search across all brains and discover what other agents have learned.

---

## Quick Start

```bash
# Install
go install github.com/bobbydeveaux/cerebra@latest

# Or build from source
git clone https://github.com/bobbydeveaux/cerebra.git
cd cerebra
go build -o cerebra .

# Index your codebase
cerebra scan ~/code/my-org

# Start watching agent conversations
cerebra brains watch

# Start MCP server (for Claude Code / Cursor)
cerebra serve

# Start web UI (wiki + brain dashboard + chat)
cerebra serve --ui
```

### Add to Claude Code

```bash
claude mcp add cerebra -- cerebra serve --db /path/to/.cerebra/jor-el.db
```

---

## Features

### Codebase Indexing (inherited from Fortress)

- **Recursive scanning** — walks directories, detects languages, discovers git repos
- **Intelligent chunking** — code by function boundaries, markdown by headings, config files kept whole
- **Incremental scans** — tracks content hashes and git SHAs, only re-embeds changed files
- **Git history indexing** — commit messages as searchable documents
- **Semantic search** — vector similarity (cosine) with FTS5 full-text fallback
- **Confluence integration** — index Confluence spaces via API

### Agent Memory (Cerebra-specific)

- **Real-time conversation watcher** — `fsnotify` monitors `~/.claude/projects/` for JSONL session files
- **Brain Registry** — every agent session tracked with metadata: project, agent type, status, last activity, summary
- **Auto summaries** — each session gets a continuously-updated summary for token-efficient retrieval
- **Cross-agent discovery** — agents find relevant context from other agents' sessions
- **Agent-agnostic** — common schema normalises across Claude Code, Cursor, Copilot, etc.

### MCP Server

First-class Model Context Protocol server over stdio. One config line to connect Claude Code, Cursor, or any MCP-compatible tool.

| Tool | Description |
|------|-------------|
| `search` | Semantic search with FTS fallback across codebase |
| `search_brain` | Search across agent conversation history |
| `list_brains` | List all known agent brains with metadata |
| `get_brain` | Get details and summary for a specific brain |
| `get_activity` | Get recent agent activity stats |
| `list_categories` | List discovered language/file categories |
| `get_document` | Retrieve a specific indexed document |
| `get_stats` | Database statistics |
| `search_agent` | Search a subagent's invocations (prompts + responses) by name and FTS query ¹ |
| `list_agent_activity` | List a subagent's invocations within a date range (metadata only) ¹ |

¹ `search_agent` and `list_agent_activity` require the `agent_messages` table (added with these tools in the same migration); run a `cerebra scan` after upgrading so agent tool-use blocks are indexed.

### Web UI

Built-in web interface powered by htmx (no JS build step):

- **Brain dashboard** — all tracked agent sessions with metadata and summaries
- **Wiki browser** — navigate indexed codebase by category
- **RAG chat** — ask questions about your codebase with retrieval-augmented answers
- **Search** — semantic + full-text search with file paths and line numbers

### Agent Meeting Mode (planned)

Structured multi-agent discussions producing permanent knowledge artefacts:

- Define an agenda with required outcomes
- Invite multiple agent brains to participate
- Each agent contributes from its own context
- Produces a wiki-ready Markdown document with decisions, risks, trade-offs, and action items
- Use cases: architecture decisions, incident reviews, security audits, sprint planning

---

## Architecture

Cerebra extends Fortress's proven indexing pipeline:

```
┌──────────────────────────────────────────────────────┐
│                       CEREBRA                         │
│                                                       │
│  Agent Memory · Brain Registry · Meeting Mode · UI    │
│                                                       │
├──────────────────────────────────────────────────────┤
│                       FORTRESS                        │
│                                                       │
│  Scanner · Chunker · Embedder · Vector Store · MCP    │
│  (codebases, Confluence, git history)                 │
└──────────────────────────────────────────────────────┘
```

### Three Runtime Modes

All modes share a single SQLite database (`.cerebra/jor-el.db`):

| Mode | Command | Purpose |
|------|---------|---------|
| Scan | `cerebra scan <path>` | Index codebases and documentation |
| Watch | `cerebra brains watch` | Monitor agent conversations in real time |
| Serve | `cerebra serve` | MCP server for AI tool integration |
| Serve + UI | `cerebra serve --ui` | Web dashboard, wiki, and chat on `:8080` |

### Key Design Decisions

- **Pure-Go SQLite** (`modernc.org/sqlite`) — cross-platform builds without CGO
- **sqlite-vec** — vector similarity search in SQLite
- **fsnotify** — native filesystem events (FSEvents on macOS, inotify on Linux)
- **htmx** — web UI with no JavaScript build step
- **Ollama** — local embeddings by default (`nomic-embed-text`), OpenAI as alternative
- **Summaries over raw context** — keep agent prompts concise, reduce token waste
- **Agent-agnostic** — common schema for any AI tool's conversation format

### Claude Code Conversation Layout

Cerebra watches this directory structure:

```
~/.claude/
├── projects/                              ← watch target
│   ├── -Users-bobby-code-project-x/       ← one dir per project
│   │   ├── <session-id>.jsonl             ← conversation log (JSONL)
│   │   ├── <session-id>/
│   │   │   └── subagents/                 ← sub-agent conversations
│   │   └── memory/                        ← Claude Code's own memory
│   └── -Users-bobby-code-project-y/
└── sessions/                              ← session index
    └── <pid>.json                         ← maps PID → session ID
```

JSONL files are append-only during a session. Cerebra tracks byte offsets and only reads new lines — making incremental indexing fast and reliable.

---

## Configuration

Default config file: `cerebra.yaml` in the working directory.

```yaml
# cerebra.yaml

# Embedding provider: "ollama" or "openai"
embedder: ollama

ollama:
  url: http://localhost:11434
  embed_model: nomic-embed-text
  chat_model: llama3.2

openai:
  api_key: ""   # or set OPENAI_API_KEY env var
  embed_model: text-embedding-3-small

# Chat LLM for RAG: ollama, openai, claude, minimax
chat_llm: openai

# Files/directories to skip
ignore:
  - .git
  - node_modules
  - vendor
  - "*.bin"
  - "*.lock"

# Chunking
chunk_size: 512
chunk_overlap: 64

# Database
db_path: .cerebra/jor-el.db

# Web UI
ui_port: 8080
ui_bind: 127.0.0.1

# Embedding concurrency
embed_workers: 4
embed_batch_size: 32
```

---

## CLI Reference

```
cerebra — codebase knowledge + agent memory for AI tools

Commands:
  scan     <path>         Scan and index a codebase
  search   <query>        Search the knowledge base
  serve                   Start MCP or web UI server
  brains   watch          Watch for agent conversations
  brains   list           List known agent brains
  stats                   Show database statistics
  watch    <path>         Watch for file changes and re-scan
  forget   <repo>         Remove a repo from the index
```

---

## Billing & Stripe (paid tier)

Cerebra's AgentOps paid tier is delivered through Stripe. The subscription
lifecycle is handled server-side by a Stripe webhook (`POST /api/stripe/webhook`).

> **Current status:** the webhook handler is shipped and verifies events, but
> paid-tier feature gating (a `RequirePaid` middleware backed by a licence
> store) is **planned, not yet wired on `main`**. All features are currently
> unrestricted regardless of subscription state. The steps below cover wiring
> the webhook so subscription events are received and verified.

### What the webhook does

The handler reads the raw request body (capped at 1 MiB), verifies the
`Stripe-Signature` header via Stripe's HMAC-SHA256 signing, and dispatches the
two events Cerebra cares about. Every other event type is accepted with a `200`
so Stripe stops retrying.

| Event | Meaning |
|-------|---------|
| `checkout.session.completed` | Subscription started |
| `customer.subscription.deleted` | Subscription ended |

Verification behaviour:

- Missing `STRIPE_WEBHOOK_SECRET` -> `500` (loud failure beats silently
  accepting any payload)
- Signature verification failure -> `400`
- Handled event processed without error -> `200 {"ok":true}`

### Environment variables

| Variable | Status | Purpose | Example |
|----------|--------|---------|---------|
| `STRIPE_WEBHOOK_SECRET` | **Required** | Signing secret used to verify the `Stripe-Signature` header. The webhook returns `500` until this is set. | `whsec_xxxxxxxxxxxxxxxxxxxxxxxx` |
| `STRIPE_PRICE_ID` | Planned | Price/plan identifier for the paid tier. Reserved for the planned `RequirePaid` gating; **not read by current code**. | `price_xxxxxxxxxxxxxxxx` |

Set these as environment variables only. On Cloud Run the value is mounted from
Secret Manager -- never hardcode it and never commit it.

### Wiring Stripe test-mode keys

1. In the [Stripe Dashboard](https://dashboard.stripe.com/test/webhooks)
   (test mode), create a webhook endpoint pointing at the Cloud Run URL:

   ```
   https://cerebra.stackramp.io/api/stripe/webhook
   ```

2. Subscribe the endpoint to at least `checkout.session.completed` and
   `customer.subscription.deleted`.
3. Copy the endpoint's **Signing secret** (begins with `whsec_`).
4. Store it as the `STRIPE_WEBHOOK_SECRET` secret in Secret Manager and expose
   it to the Cloud Run service as an environment variable of the same name.
5. Use Stripe's "Send test webhook" or the [Stripe CLI](https://stripe.com/docs/stripe-cli)
   (`stripe listen --forward-to localhost:8080/api/stripe/webhook`) to confirm
   events verify and return `200`.

Local development uses the same variable: export `STRIPE_WEBHOOK_SECRET` in your
shell (or the Stripe CLI's printed `whsec_` value when using `stripe listen`)
before starting `cerebra serve --ui`.

---

## Eval harness

Cerebra ships a deterministic, API-free retrieval benchmark that gates every
pull request. It proves the FTS keyword search path keeps surfacing the right
facts as the codebase changes -- no embeddings, no Ollama, no OpenAI, no
network, and no API keys, so it runs identically on a laptop and in CI.

> The harness is distinct from the live LLM ablation in `evals/run.sh`, which
> spawns real `claude -p` sessions and grades answers with an LLM judge. That
> benchmark needs API keys and a private corpus, so it cannot gate CI. The
> harness below is the one wired into the pipeline.

### What it does

1. Seeds an ephemeral SQLite database (`sqlite_fts5`) from a small,
   self-contained fixture corpus embedded in the binary via `go:embed`.
2. Runs each question's query through the FTS retrieval path
   (`SearchFTS`) and inspects the top-N results.
3. Passes a question when every one of its expected terms appears
   (case-insensitively) somewhere in those results, and reports the
   overall pass rate.

A run exits 0 when the pass rate clears the threshold (default 70%) and
non-zero otherwise, printing a per-question PASS/FAIL list and a summary line
to stdout.

### Running locally

```bash
# Unit tests for the harness (seed + run + threshold logic)
go test ./internal/eval/...

# Full CI-mode run against an ephemeral database
make build                       # builds with the sqlite_fts5 tag
./cerebra eval --ci --threshold 0.70 --top-n 5
```

`--ci` is currently the only supported mode: it creates a temporary database,
seeds the embedded fixtures, runs the suite, and exits non-zero below the
threshold. `--threshold` (default `0.70`) sets the minimum pass rate and
`--top-n` (default `5`) bounds how many FTS results are inspected per question.

Example output:

```
  [PASS] C01 difference between Cerebra and Fortress fork
  [PASS] C02 token counting per brain double counting bug
  ...
eval: 7/7 passed (100%), threshold 70%
```

### CI gate

`.github/workflows/evals.yml` runs the suite on every push to `main`, every
pull request, and on manual dispatch. The job builds with `make build` (so the
`chunks_fts` index exists) and then runs `./cerebra eval --ci --threshold
0.70`. A pass rate below 70% fails the check and blocks the merge.

### Fixtures and adding a question

The fixture corpus lives in `internal/eval/fixtures/*.md` and is embedded at
build time. Each fixture is a short Markdown document ingested through the
normal `UpsertDocument` path, one chunk per file. The question set is defined
by `Questions()` in `internal/eval/eval.go`. To add a check, ensure the fact
you want to assert is present in a fixture (extend an existing file or add a
new one), then append a `Question` with its `Query` and the `MustContain`
terms the top-N results should surface.

---

## Running After Reboot

```bash
# Start the brains watcher (background)
nohup cerebra brains watch --db-path ~/.cerebra/jor-el.db > /tmp/cerebra-brains-watch.log 2>&1 &

# MCP server is managed by Claude Code — just reconnect via /mcp

# Verify
ps aux | grep "[c]erebra"
```

---

## Development

```bash
# Build
go build -o cerebra .

# Run tests
go test ./...

# Start dev server for the marketing site
cd site && npm run dev
```

---

## Strategic Vision

Cerebra transforms a codebase indexer into a **persistent AI engineering memory system**:

```
  Codebase knowledge        (scan pipeline)
+ Documentation knowledge   (Confluence integration)
+ Conversation knowledge    (brain watcher)
+ Agent activity knowledge  (brain registry)
+ Agent collaboration       (meeting mode)
─────────────────────────────────────────────
= Engineering Brain
```

**MVP success criteria:** A new Claude Code session can discover useful context from a previous session — without manually copying prompts or summaries.

---

## License

MIT

---

Built by engineers who got tired of starting from zero.
