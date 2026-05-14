import { Link } from 'react-router-dom'
import DocPage from '../DocPage.jsx'

export default function Overview() {
  return (
    <DocPage slug="">
      <p>
        <strong>Fortress</strong> is an open-source CLI tool that scans your codebase, generates embeddings,
        and stores them in a local SQLite vector database. It gives every AI tool your engineers use &mdash;
        Claude Code, Copilot, Cursor, Windsurf &mdash; deep, searchable context about your entire codebase.
      </p>

      <h2>Why Fortress?</h2>
      <p>
        AI coding assistants are powerful, but they're flying blind. They don't know your architecture,
        your naming conventions, or how your services connect. Fortress fixes that by creating a knowledge
        base that any AI tool can query via the <strong>Model Context Protocol (MCP)</strong>.
      </p>

      <div className="doc-grid">
        <div className="doc-grid-item">Scan 20,000+ files in minutes</div>
        <div className="doc-grid-item">100% local &mdash; your code never leaves your machine</div>
        <div className="doc-grid-item">Native MCP server for Claude Code</div>
        <div className="doc-grid-item">Web UI with wiki browser + RAG chat</div>
        <div className="doc-grid-item">Incremental scans &mdash; only re-embeds changed files</div>
        <div className="doc-grid-item">CI/CD action for automatic reindexing</div>
      </div>

      <h2>How it works</h2>
      <ol>
        <li><strong>Scan</strong> &mdash; Point Fortress at one or more repos. It detects languages, chunks code by function boundaries, and generates embeddings.</li>
        <li><strong>Store</strong> &mdash; Embeddings are stored in a local SQLite database with <code>sqlite-vec</code> for vector search and FTS5 for full-text search.</li>
        <li><strong>Serve</strong> &mdash; Expose the knowledge base via MCP (for AI tools) or a web UI (for humans).</li>
      </ol>

      <h2>Get started</h2>
      <div className="doc-cards">
        <Link to="/docs/installation" className="doc-link-card">
          <h4>Installation</h4>
          <p>Install Fortress in under a minute</p>
        </Link>
        <Link to="/docs/quickstart" className="doc-link-card">
          <h4>Quick Start</h4>
          <p>Scan, search, and serve in 3 commands</p>
        </Link>
        <Link to="/docs/mcp-server" className="doc-link-card">
          <h4>MCP Server</h4>
          <p>Connect Claude Code to your codebase</p>
        </Link>
        <Link to="/docs/ci-cd" className="doc-link-card">
          <h4>CI/CD Integration</h4>
          <p>Keep the index fresh automatically</p>
        </Link>
      </div>
    </DocPage>
  )
}
