import DocPage, { Code, Table } from '../DocPage.jsx'

export default function CliReference() {
  return (
    <DocPage slug="cli-reference">
      <h2>cerebra scan</h2>
      <p>Recursively scan directories, generate embeddings, and store in the database.</p>
      <Code>{`cerebra scan <path> [path2] [path3] ...`}</Code>
      <Table
        headers={['Flag', 'Default', 'Description']}
        rows={[
          [<code>--db</code>, '.cerebra/jor-el.db', 'Path to SQLite database'],
          [<code>--config</code>, 'cerebra.yaml', 'Path to config file'],
        ]}
      />

      <h2>cerebra search</h2>
      <p>Search the indexed knowledge base.</p>
      <Code>{`cerebra search "your query here"`}</Code>
      <Table
        headers={['Flag', 'Default', 'Description']}
        rows={[
          [<code>--limit</code>, '10', 'Number of results to return'],
          [<code>--db</code>, '.cerebra/jor-el.db', 'Path to SQLite database'],
        ]}
      />

      <h2>cerebra serve</h2>
      <p>Start the MCP server (stdio) or web UI (HTTP).</p>
      <Code>{`# MCP server (for Claude Code, Cursor)
cerebra serve

# Web UI
cerebra serve --ui`}</Code>
      <Table
        headers={['Flag', 'Default', 'Description']}
        rows={[
          [<code>--ui</code>, 'false', 'Start web UI instead of MCP server'],
          [<code>--db</code>, '.cerebra/jor-el.db', 'Path to SQLite database'],
          [<code>--port</code>, '8080', 'Web UI port'],
          [<code>--bind</code>, '127.0.0.1', 'Web UI bind address'],
        ]}
      />

      <h2>cerebra stats</h2>
      <p>Show database statistics.</p>
      <Code>{`cerebra stats`}</Code>

      <h2>cerebra watch</h2>
      <p>Watch for file changes and automatically re-scan.</p>
      <Code>{`cerebra watch <path>`}</Code>

      <h2>cerebra forget</h2>
      <p>Remove a repo from the index.</p>
      <Code>{`cerebra forget <repo-name>`}</Code>

      <h2>cerebra brains</h2>
      <p>Manage tracked agent sessions (brains). Cerebra watches for AI agent conversation files and indexes them for cross-agent discovery.</p>

      <h3>cerebra brains watch</h3>
      <p>Watch for new or modified agent conversation files and index them automatically.</p>
      <Code>{`cerebra brains watch`}</Code>
      <p>
        Monitors <code>~/.claude/projects/*/</code> for new or updated <code>*.jsonl</code> conversation files.
        When changes are detected, conversations are parsed, chunked, embedded, and stored in the database.
      </p>

      <h3>cerebra brains list</h3>
      <p>List all known agent sessions with metadata and last activity.</p>
      <Code>{`cerebra brains list`}</Code>
      <Table
        headers={['Flag', 'Default', 'Description']}
        rows={[
          [<code>--limit</code>, '20', 'Number of brains to display'],
          [<code>--db</code>, '.cerebra/jor-el.db', 'Path to SQLite database'],
        ]}
      />

      <h2>Global flags</h2>
      <Table
        headers={['Flag', 'Description']}
        rows={[
          [<code>--config</code>, 'Path to cerebra.yaml config file'],
          [<code>--db</code>, 'Path to SQLite database file'],
          [<code>--help</code>, 'Show help for any command'],
        ]}
      />
    </DocPage>
  )
}
