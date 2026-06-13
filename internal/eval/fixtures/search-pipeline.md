# Search Pipeline

Cerebra indexes content through a pipeline of scanner, chunker, embedder, and
store. The store is backed by SQLite with the FTS5 full-text extension.

Two retrieval paths exist. Vector search compares an embedded query against
stored embeddings. FTS search is a keyword path that matches query terms against
the chunks_fts virtual table, requiring no embeddings and no external API.

When vector search returns no results, the search command falls back to FTS so a
query still returns relevant chunks.
