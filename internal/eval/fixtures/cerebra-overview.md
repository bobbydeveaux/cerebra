# Cerebra Overview

Cerebra is a Go CLI tool forked from Fortress that combines codebase knowledge
indexing with persistent AI agent memory. It scans codebases, indexes AI agent
conversations, generates embeddings, and stores them in a local SQLite vector
database called Jor-El.

Cerebra is queryable three ways: an MCP server for Claude Code, a command-line
interface, and a web UI that serves a wiki and a chat page.

The difference between Cerebra and Fortress is that Cerebra is a fork of Fortress
that adds a conversation watcher, an agent brain registry, and a summary layer on
top of the inherited scan, embed, and MCP pipeline. Fortress remains a pure
codebase and Confluence indexer.
