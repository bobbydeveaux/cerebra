import DocPage, { Code, Callout } from '../DocPage.jsx'

export default function QuickStart() {
  return (
    <DocPage slug="quickstart">
      <p>
        Get from zero to a fully searchable codebase &mdash; with agent memory &mdash; in a few commands.
      </p>

      <h2>1. Scan your codebase</h2>
      <p>Point Cerebra at any directory. It recursively discovers repos, detects languages, chunks code intelligently, and generates embeddings.</p>
      <Code>{`# Scan a single repo
cerebra scan ./my-project

# Scan a directory of repos (monorepo or multi-repo)
cerebra scan ./all-repos`}</Code>

      <p>You'll see a live progress bar as files are processed:</p>
      <Code>{`Scanning ./all-repos...
  [████████████████████████████████] 20,552/20,552 files

Done!
  Repos:   110
  Files:   20,552
  Chunks:  122,046
  DB size: 1,418 MB`}</Code>

      <Callout type="info">
        <strong>Incremental by default.</strong> Cerebra tracks content hashes and git SHAs.
        Re-running <code>cerebra scan</code> only processes changed files.
      </Callout>

      <h2>2. Watch agent conversations</h2>
      <p>
        Start the brains watcher to monitor AI agent conversation files. Cerebra will automatically
        index new and updated sessions, maintain a brain registry, and generate summaries.
      </p>
      <Code>{`# Start watching agent conversations (e.g. Claude Code JSONL sessions)
cerebra brains watch`}</Code>

      <Callout type="info">
        <strong>Cross-agent discovery.</strong> Once conversations are indexed, any new agent session
        can discover useful context from previous sessions without manually copying prompts or summaries.
      </Callout>

      <h2>3. Search</h2>
      <p>Search your indexed codebase using natural language:</p>
      <Code>{`# Semantic search
cerebra search "how does authentication work"

# Full-text search fallback
cerebra search "handleLogin"`}</Code>

      <h2>4. Serve</h2>
      <p>Expose your knowledge base to AI tools or browse it in a web UI:</p>
      <Code>{`# Start MCP server (for Claude Code, Cursor, etc.)
cerebra serve

# Start web UI (wiki + RAG chat + brain dashboard)
cerebra serve --ui`}</Code>

      <h2>What's next?</h2>
      <ul>
        <li><a href="/docs/mcp-server">Add Cerebra as an MCP server</a> in Claude Code for AI-powered codebase queries</li>
        <li><a href="/docs/brains">Explore the Brain Registry</a> to manage and search agent sessions</li>
        <li><a href="/docs/web-ui">Explore the Web UI</a> with wiki browser, RAG chat, and brain dashboard</li>
        <li><a href="/docs/configuration">Configure</a> embedding providers, ignore patterns, and more</li>
        <li><a href="/docs/ci-cd">Set up CI/CD</a> to keep your index fresh on every commit</li>
      </ul>
    </DocPage>
  )
}
