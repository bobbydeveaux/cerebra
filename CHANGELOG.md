# Changelog

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
