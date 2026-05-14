import { Link } from 'react-router-dom'
import DocPage from '../DocPage.jsx'

export default function Overview() {
  return (
    <DocPage slug="">
      <p>
        <strong>Cerebra</strong> is an open-source CLI tool that combines codebase knowledge indexing with
        persistent AI agent memory. It scans your codebase, indexes AI agent conversations, generates
        embeddings, and stores them in a local SQLite vector database. It gives every AI tool your
        engineers use &mdash; Claude Code, Copilot, Cursor, Windsurf &mdash; deep, searchable context
        about your entire codebase <em>and</em> the history of every agent session.
      </p>

      <h2>Why Cerebra?</h2>
      <p>
        AI coding assistants are powerful, but they're flying blind. They don't know your architecture,
        your naming conventions, or what another agent session already figured out yesterday. Cerebra
        fixes that by creating a knowledge base that any AI tool can query via the{' '}
        <strong>Model Context Protocol (MCP)</strong> &mdash; covering both your code and your agents' collective memory.
      </p>

      <div className="doc-grid">
        <div className="doc-grid-item">Scan 20,000+ files in minutes</div>
        <div className="doc-grid-item">100% local &mdash; your code never leaves your machine</div>
        <div className="doc-grid-item">Native MCP server for Claude Code</div>
        <div className="doc-grid-item">Web UI with wiki browser, RAG chat + brain dashboard</div>
        <div className="doc-grid-item">Incremental scans &mdash; only re-embeds changed files</div>
        <div className="doc-grid-item">Agent brain registry &mdash; track and search all agent sessions</div>
        <div className="doc-grid-item">Cross-agent discovery &mdash; agents find context from other sessions</div>
        <div className="doc-grid-item">CI/CD action for automatic reindexing</div>
      </div>

      <h2>How it works</h2>
      <ol>
        <li><strong>Scan</strong> &mdash; Point Cerebra at one or more repos. It detects languages, chunks code by function boundaries, and generates embeddings.</li>
        <li><strong>Watch</strong> &mdash; Cerebra monitors agent conversation files (e.g. Claude Code JSONL sessions), indexes them incrementally, and maintains a brain registry with summaries for each session.</li>
        <li><strong>Store</strong> &mdash; Embeddings are stored in a local SQLite database with <code>sqlite-vec</code> for vector search and FTS5 for full-text search.</li>
        <li><strong>Serve</strong> &mdash; Expose the knowledge base via MCP (for AI tools) or a web UI (for humans). Agents can search both code and other agents' context.</li>
      </ol>

      <h2>Get started</h2>
      <div className="doc-cards">
        <Link to="/docs/installation" className="doc-link-card">
          <h4>Installation</h4>
          <p>Install Cerebra in under a minute</p>
        </Link>
        <Link to="/docs/quickstart" className="doc-link-card">
          <h4>Quick Start</h4>
          <p>Scan, search, and serve in a few commands</p>
        </Link>
        <Link to="/docs/mcp-server" className="doc-link-card">
          <h4>MCP Server</h4>
          <p>Connect Claude Code to your codebase and agent memory</p>
        </Link>
        <Link to="/docs/brains" className="doc-link-card">
          <h4>Brains</h4>
          <p>Explore the agent brain registry and cross-agent discovery</p>
        </Link>
      </div>
    </DocPage>
  )
}
