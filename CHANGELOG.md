# Changelog

## [Unreleased]

### Added
- **RequirePaid paid-tier gating** (agentops-090) - wires the `RequirePaid` middleware around the premium endpoints `POST /api/search`, `GET /api/chat/stream` and `GET /api/brains/{id}`. Subscription state is persisted in a new SQLite `subscriptions` table written by the Stripe webhook (`checkout.session.completed` activates, `customer.subscription.deleted` deactivates), replacing the no-op `loggingStripeHandler` with a store-backed `storeStripeHandler`. The gate fails open unless `STRIPE_WEBHOOK_SECRET` is set (local dev + eval CI unaffected) and returns `402 Payment Required` with a JSON `{error, checkout_url}` body sourced from `STRIPE_CHECKOUT_URL`. Instance-level: any active subscription unlocks the instance (Cerebra is single-user local-first; `stackramp.yaml` sets `database: false`).
- **Stripe webhook handler** (agentops-011, ad78fe8) — `POST /api/stripe/webhook` reads the raw request body (1 MiB cap), verifies the `Stripe-Signature` header via `stripe-go`'s `webhook.ConstructEventWithOptions`, and dispatches `checkout.session.completed` and `customer.subscription.deleted` to a `StripeEventHandler` interface. Missing `STRIPE_WEBHOOK_SECRET` fails loud with 500; signature failures return 400; unknown event types are accepted with 200 so Stripe stops retrying. Dependency: `github.com/stripe/stripe-go/v76 v76.25.0`.
- **Agent-level indexing** (agentops-010, 65ba86e) — new `agent_messages` table (PK = `tool_use_id`) plus FTS5 mirror parses Agent tool-use blocks (`subagent_type`, `prompt`, `description`) from assistant messages and stitches them to the matching tool-result block via `tool_use_id`. Exposes two new MCP tools — `search_agent` and `list_agent_activity` — so queries like "what did Marcus flag this month?" no longer require semantic search across all brains.
- **Test workflow with `sqlite_fts5` build tag** — GitHub Actions runs `go test ./...` with the build tag required for the FTS5 search path (#4)
- **`internal/web` test suite** — handler coverage raised from 7.7% to 83.9%, exercising wiki, search, and chat handlers (#5)
- **`internal/brain` test suite** — registry, session loading, and summary paths covered, 0% to 78.1% (#6)
- **`internal/scanner` test suite** — language detection, categorisation, and walk logic covered, 25% to 91% (#7)
- **`subscriptions` store error-path tests** (agentops-109) — `TestSubscription_ClosedDBErrors` drives `SetSubscriptionActive`, `SetSubscriptionInactive`, `HasActiveSubscription` and `GetSubscription` against a closed DB, covering the previously untested `fmt.Errorf` wrap in each method. All four functions in `subscriptions.go` now report 100% statement coverage. Follows the closed-DB convention of `TestActivity_ClosedDBErrors` and `TestAgentMessages_ClosedDBErrors`.
- **`internal/store` Search and GetDocument path tests** (agentops-111) — extends `store_test.go` with five focused tests covering paths previously left without assertions: `GetDocument` not-found (bare `sql.ErrNoRows` on a populated DB) and absolute-`path` lookup (the `OR path = ?` branch), metadata JSON round-trip with multi-chunk ordering by `start_line` and parent-doc `ChunkMeta` population, and `Search` distance ranking with limit truncation plus the empty-index empty-slice case. `GetDocument` 90.9%, `Search` 85.7%; package holds at 88.5%. Test-only, no production change.
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
- **`internal/brain/watcher` test suite** — `Start`, `addWatchRecursive`, and `handleEvent` covered against real `*fsnotify.Watcher` instances over temp directories plus synthetic events, raising those functions from 0% to 70.8/85.7/92.0% and the package total from 78.1% to 90.4% (#29)
- **CHANGELOG `[Unreleased]` backfilled for agentops-010 + agentops-011** — agent-level indexing (65ba86e) and Stripe webhook handler (ad78fe8) entries added at the top of `[Unreleased]` in-format, after both had landed on main without CHANGELOG coverage (#30)
- **`cmd/` runner coverage to ≥70%** — four hermetic tests in `cmd/cmd_runners_test.go` lift `runSearch` 48.6% to 86.5%, `runServe` 46.2% to 88.5%, `runWatch` 36.0% to 73.3%, and `runBrainsWatch` 64.7% to 82.4%; `cmd/` package total 64.3% to 77.0% via test-only changes (#31)
- **README Stripe billing section** (agentops-064) -- documents the shipped Stripe webhook handler (`POST /api/stripe/webhook`), the required `STRIPE_WEBHOOK_SECRET` env var (and the planned-only `STRIPE_PRICE_ID`), the test-mode key wiring steps, and the Cloud Run webhook URL (`https://cerebra.stackramp.io/api/stripe/webhook`). States explicitly that paid-tier `RequirePaid` gating is planned but not yet wired on `main`, so Bobby has accurate context when wiring keys (agentops-016). Docs-only, no code changes.
- **CHANGELOG `[Unreleased]` backfilled for PRs #29-#31** — watcher test suite (#29), the agentops-010/agentops-011 backfill (#30), and the `cmd/` runner coverage entry (#31) documented in-format (#32).
- **README MCP tools table extended with `search_agent` + `list_agent_activity`** (agentops-067, 1f94e76) — adds the two agent-level MCP tools to the README tools table (worded from the server tool descriptions) plus a footnote noting the `agent_messages` migration requires a re-scan, taking the documented tool count from 8 to 10. Docs-only, no code changes (#34).
- **API-free retrieval eval suite + CI gate** (agentops-072, 8af3dd0) — new `internal/eval` package with `go:embed` self-contained fixtures, a deterministic FTS-only `Run` over `SearchFTS` with a case-insensitive top-N `MustContain` check, and a `cerebra eval --ci --threshold --top-n` command that exits non-zero below threshold. A `.github/workflows/evals.yml` Eval Suite job builds with the `sqlite_fts5` tag and runs the gate at `--threshold 0.70`, so retrieval quality is enforced in CI without external API keys (#35).
- **README eval harness section** (agentops-073, 1437aa4) — documents what the eval harness does, how to run it locally (`go test ./internal/eval/...` and `cerebra eval --ci`), the pass/fail semantics, the `evals.yml` CI gate, and how to add a fixture and question. Docs-only, no code changes (#36).
- **`internal/store` FTS tests skip gracefully without the `sqlite_fts5` tag** (agentops-075, 78bdca4) — the `chunks_fts` and `agent_messages_fts` virtual tables only exist in fts5-tagged builds, so plain `go test ./internal/store/` failed four tests. Each FTS-dependent test now guards on the existing `SQLiteStore.ftsAvailable` flag and `t.Skip`s with a message pointing to `make test` (which sets `-tags sqlite_fts5`); untagged runs exit 0 with four clean skips while `make test` keeps running and passing all four. Test files only, no production change (#38).
- **`internal/rag` httptest tests skip when port binding is unavailable** (agentops-077, b40f918) — in a no-network sandbox `httptest.NewServer` panics rather than erroring when it cannot bind a local port, aborting the whole package. A new unexported `requireNetwork(t)` helper probes `net.Listen` on `127.0.0.1:0` and calls `t.Skip` when binding is denied; it runs first in the 12 tests that construct an `httptest.Server`. Under full networking (CI / `make test`) the probe is a no-op and every test runs unchanged. Test files only, no production change (#39).
- **README Eval Suite CI badge** (agentops-076, f31efc2) — adds the Eval Suite status badge to the README, linking the `.github/workflows/evals.yml` workflow so the API-free retrieval gate's pass/fail state is visible from the project landing page. Docs-only, no code changes (#40).
- **CHANGELOG `[Unreleased]` backfilled for PRs #38-#40** (agentops-078, 954e1c9) — the FTS-tag-guarded `internal/store` skips (#38), the `internal/rag` network-probe skips (#39), and the README Eval Suite CI badge (#40) documented in-format after they had landed on main without CHANGELOG coverage. Docs-only, no code changes (#41).
- **`cmd/` eval runner test coverage** (agentops-079, fa50f78) — new `cmd/` tests cover the `runEval --ci` pass and fail paths, the guard condition, and the below-threshold branch, restoring the `cmd/` package to the ≥70% coverage floor after PR #35 added `eval.go` without tests. Test files only, no production change (#42).
- **CHANGELOG `[Unreleased]` backfilled for PRs #41 and #42** (agentops-082, bb723e6) — the FTS-tag-guarded skips / network-probe skips / README Eval Suite CI badge backfill (#41) and the `cmd/` eval runner coverage entry (#42) documented in-format after they had landed on main. Docs-only, no code changes (#43).
- **`internal/eval` Seed/Run error-path + Meets boundary tests** (agentops-084, 55ae9aa) — extends the eval suite with coverage for `Seed` failure handling, `Run` error paths, and the `Meets` threshold boundary condition, hardening the API-free retrieval gate against regressions. Test files only, no production change (#44).
- **`internal/mcp` malformed-args + scanner-overflow tests; nil-document guard** (agentops-088, ea466bc) — table-driven tests across the MCP server's arg-taking tools assert that malformed and omitted arguments degrade to zero values rather than panicking, plus a scanner buffer-overflow path; a surfaced latent SIGSEGV in `toolGetDocument` (nil `*Document` dereference when the store returned a nil doc) is fixed with a `-32000` nil guard mirroring `toolGetBrain`. Brings `internal/mcp` to 99.2% coverage (#45).
- **CHANGELOG `[Unreleased]` backfilled for PRs #43, #44, and #45** (agentops-089, 442fabd) — the FTS+rag+store skips / eval Seed-Run error-path tests / MCP malformed-args + nil-doc guard documented in-format after they had landed on main without CHANGELOG coverage. Docs-only, no code changes (#46).

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
