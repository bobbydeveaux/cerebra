# Cerebra Startup

## MCP Server

Managed by Claude Code — just run `/mcp` and reconnect. No manual start needed.

## Brains Watcher

Monitors `~/.claude/projects/` for JSONL conversation files, indexes them, and populates the `brain_activity` table for hourly stats.

```bash
nohup ~/code/newlife/cerebra/cerebra brains watch --db-path ~/code/newlife/cerebra/.cerebra/jor-el.db > /tmp/cerebra-brains-watch.log 2>&1 &
```

## Verify

```bash
ps aux | grep "[c]erebra"
```

Should show both `cerebra serve` and `cerebra brains watch`.

## Rebuild (after code changes)

**Always use `make build`** — never plain `go build`. The Makefile passes the `sqlite_fts5` tag, without which **both codebase FTS (`chunks_fts`) and agent-invocation FTS (`agent_messages_fts`) silently fail to create** and search falls back to slow LIKE queries.

```bash
cd ~/code/newlife/cerebra && make build
```

Equivalent to: `go build -tags "sqlite_fts5" -o cerebra .`

Then kill and restart the watcher. The MCP server picks up the new binary on next `/mcp` reconnect.

## FTS5 build tag — why it matters

Cerebra uses `mattn/go-sqlite3` (CGO). That driver gates FTS5 behind a build tag. If you build without it, the binary runs fine but:

- `CREATE VIRTUAL TABLE ... USING fts5(...)` returns "no such module: fts5"
- The schema init swallows the error and continues
- Search degrades silently to LIKE queries
- The breakage is invisible until you actually try FTS

Symptom check: run `sqlite3 .cerebra/jor-el.db "SELECT name FROM sqlite_master WHERE type='table' AND name LIKE '%fts%'"`. If the result is empty, you've built without the tag — rerun `make build`.

## Log

```bash
tail -f /tmp/cerebra-brains-watch.log
```

## Confluence indexing (optional datasource)

Cerebra ships a Confluence Cloud v1 REST connector at `internal/datasource/confluence/` (see `docs/LLD.md` §14.2). It pulls pages from configured spaces and feeds them through the same chunker → embedder → store pipeline as filesystem content.

### Configuration

```yaml
# cerebra.yaml
confluence:
  base_url: https://<your-org>.atlassian.net/wiki
  email: <atlassian-user-email>
  api_token: ""              # leave blank, set via env var
  space_keys:                # optional; empty = index all spaces the token can read
    - RES
```

### Environment variables

The API token is resolved in this order (first non-empty wins):

| Order | Variable | Notes |
|---|---|---|
| 1 | `confluence.api_token` in `cerebra.yaml` | Don't commit real tokens to the file |
| 2 | `TT_RES_CONFLUENCE` | Toucanberry-specific legacy name; kept for backwards compatibility |
| 3 | `CONFLUENCE_API_TOKEN` | Canonical name; prefer this for new setups |

Generate a token at <https://id.atlassian.com/manage-profile/security/api-tokens>. It needs read access to every space listed under `space_keys`.

### One-shot scan

```bash
export CONFLUENCE_API_TOKEN=your-token-here
make build
./cerebra scan      # picks up the confluence: block from cerebra.yaml
```

If the connector returns HTTP 401, the token is missing or wrong; HTTP 403 means the token user does not have read access to one of the configured spaces.
