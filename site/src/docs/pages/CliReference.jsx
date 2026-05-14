import DocPage, { Code, Table } from '../DocPage.jsx'

export default function CliReference() {
  return (
    <DocPage slug="cli-reference">
      <h2>fortress scan</h2>
      <p>Recursively scan directories, generate embeddings, and store in the database.</p>
      <Code>{`fortress scan <path> [path2] [path3] ...`}</Code>
      <Table
        headers={['Flag', 'Default', 'Description']}
        rows={[
          [<code>--db</code>, '.fortress/jor-el.db', 'Path to SQLite database'],
          [<code>--config</code>, 'fortress.yaml', 'Path to config file'],
        ]}
      />

      <h2>fortress search</h2>
      <p>Search the indexed knowledge base.</p>
      <Code>{`fortress search "your query here"`}</Code>
      <Table
        headers={['Flag', 'Default', 'Description']}
        rows={[
          [<code>--limit</code>, '10', 'Number of results to return'],
          [<code>--db</code>, '.fortress/jor-el.db', 'Path to SQLite database'],
        ]}
      />

      <h2>fortress serve</h2>
      <p>Start the MCP server (stdio) or web UI (HTTP).</p>
      <Code>{`# MCP server (for Claude Code, Cursor)
fortress serve

# Web UI
fortress serve --ui`}</Code>
      <Table
        headers={['Flag', 'Default', 'Description']}
        rows={[
          [<code>--ui</code>, 'false', 'Start web UI instead of MCP server'],
          [<code>--db</code>, '.fortress/jor-el.db', 'Path to SQLite database'],
          [<code>--port</code>, '8080', 'Web UI port'],
          [<code>--bind</code>, '127.0.0.1', 'Web UI bind address'],
        ]}
      />

      <h2>fortress stats</h2>
      <p>Show database statistics.</p>
      <Code>{`fortress stats`}</Code>

      <h2>fortress watch</h2>
      <p>Watch for file changes and automatically re-scan.</p>
      <Code>{`fortress watch <path>`}</Code>

      <h2>fortress forget</h2>
      <p>Remove a repo from the index.</p>
      <Code>{`fortress forget <repo-name>`}</Code>

      <h2>Global flags</h2>
      <Table
        headers={['Flag', 'Description']}
        rows={[
          [<code>--config</code>, 'Path to fortress.yaml config file'],
          [<code>--db</code>, 'Path to SQLite database file'],
          [<code>--help</code>, 'Show help for any command'],
        ]}
      />
    </DocPage>
  )
}
