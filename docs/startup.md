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

```bash
cd ~/code/newlife/cerebra && go build -o cerebra .
```

Then kill and restart the watcher. The MCP server picks up the new binary on next `/mcp` reconnect.

## Log

```bash
tail -f /tmp/cerebra-brains-watch.log
```
