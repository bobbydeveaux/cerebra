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
