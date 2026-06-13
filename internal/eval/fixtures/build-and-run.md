# Build and Run

Build Cerebra with the sqlite_fts5 build tag, because the FTS5 full-text index
is gated behind that tag. Without the tag the chunks_fts table is never created
and search silently falls back to slow LIKE queries.

Run cerebra scan to ingest a directory, cerebra serve to start the MCP stdio
server for Claude Code, and cerebra serve with the ui flag to start the web UI
on port 8080.
