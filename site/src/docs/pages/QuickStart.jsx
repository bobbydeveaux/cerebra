import DocPage, { Code, Callout } from '../DocPage.jsx'

export default function QuickStart() {
  return (
    <DocPage slug="quickstart">
      <p>
        Get from zero to a fully searchable codebase in three commands.
      </p>

      <h2>1. Scan your codebase</h2>
      <p>Point Fortress at any directory. It recursively discovers repos, detects languages, chunks code intelligently, and generates embeddings.</p>
      <Code>{`# Scan a single repo
fortress scan ./my-project

# Scan a directory of repos (monorepo or multi-repo)
fortress scan ./all-repos`}</Code>

      <p>You'll see a live progress bar as files are processed:</p>
      <Code>{`Scanning ./all-repos...
  [████████████████████████████████] 20,552/20,552 files

Done!
  Repos:   110
  Files:   20,552
  Chunks:  122,046
  DB size: 1,418 MB`}</Code>

      <Callout type="info">
        <strong>Incremental by default.</strong> Fortress tracks content hashes and git SHAs.
        Re-running <code>fortress scan</code> only processes changed files.
      </Callout>

      <h2>2. Search</h2>
      <p>Search your indexed codebase using natural language:</p>
      <Code>{`# Semantic search
fortress search "how does authentication work"

# Full-text search fallback
fortress search "handleLogin"`}</Code>

      <h2>3. Serve</h2>
      <p>Expose your knowledge base to AI tools or browse it in a web UI:</p>
      <Code>{`# Start MCP server (for Claude Code, Cursor, etc.)
fortress serve

# Start web UI (wiki + RAG chat)
fortress serve --ui`}</Code>

      <h2>What's next?</h2>
      <ul>
        <li><a href="/docs/mcp-server">Add Fortress as an MCP server</a> in Claude Code for AI-powered codebase queries</li>
        <li><a href="/docs/web-ui">Explore the Web UI</a> with wiki browser and RAG chat</li>
        <li><a href="/docs/configuration">Configure</a> embedding providers, ignore patterns, and more</li>
        <li><a href="/docs/ci-cd">Set up CI/CD</a> to keep your index fresh on every commit</li>
      </ul>
    </DocPage>
  )
}
