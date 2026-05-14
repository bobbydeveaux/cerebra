import DocPage, { Code, Callout, Table } from '../DocPage.jsx'

export default function McpServer() {
  return (
    <DocPage slug="mcp-server">
      <p>
        The MCP (Model Context Protocol) server is how Fortress connects to AI tools like
        <strong> Claude Code</strong>, <strong>Cursor</strong>, and any other MCP-compatible client.
        It runs over <strong>stdio</strong> using JSON-RPC 2.0.
      </p>

      <h2>Add to Claude Code</h2>
      <p>
        Register Fortress as an MCP server in Claude Code with a single command:
      </p>
      <Code>{`claude mcp add fortress -- fortress serve --db /path/to/.fortress/jor-el.db`}</Code>

      <Callout type="info">
        <strong>The <code>--</code> separator is important.</strong> It tells the CLI that
        everything after it is the command to run, not flags for <code>claude mcp add</code>.
      </Callout>

      <p>
        If your database is in the current directory's <code>.fortress/</code> folder, you can omit the <code>--db</code> flag:
      </p>
      <Code>{`claude mcp add fortress -- fortress serve`}</Code>

      <h2>Verify it's working</h2>
      <p>After adding the MCP server, Claude Code can use Fortress tools in conversation:</p>
      <Code>{`You: "How does the authentication middleware work in our codebase?"

Claude: I'll search the Fortress knowledge base for that.
[Calls fortress.search with query "authentication middleware"]

Based on the codebase, the auth middleware works as follows:
- [1] In api/middleware/auth.go (lines 15-42), the AuthMiddleware
  function validates JWT tokens...`}</Code>

      <h2>Available tools</h2>
      <p>The MCP server exposes four tools that AI clients can call:</p>
      <Table
        headers={['Tool', 'Description', 'Parameters']}
        rows={[
          [
            <code>search</code>,
            'Semantic search with FTS fallback across the indexed codebase',
            <><code>query</code> (string, required), <code>limit</code> (int, default: 10)</>
          ],
          [
            <code>list_categories</code>,
            'List all language/file categories discovered during scanning',
            'None'
          ],
          [
            <code>get_document</code>,
            'Retrieve a specific indexed document by file path',
            <><code>path</code> (string, required)</>
          ],
          [
            <code>get_stats</code>,
            'Get statistics: repo count, file count, chunk count, DB size',
            'None'
          ],
        ]}
      />

      <h2>How search works</h2>
      <p>When an AI tool calls the <code>search</code> tool:</p>
      <ol>
        <li>The query is embedded using your configured embedding provider (Ollama or OpenAI)</li>
        <li>Vector similarity search finds the top-k matching chunks</li>
        <li>If no vector results are found, full-text search (FTS5) is used as a fallback</li>
        <li>Results are returned with content, file path, line numbers, category, and similarity score</li>
      </ol>

      <h2>Add to other MCP clients</h2>
      <p>
        Any MCP-compatible client can use Fortress. The server communicates over <strong>stdio</strong> &mdash;
        launch it with <code>fortress serve</code> and pipe JSON-RPC messages through stdin/stdout.
      </p>
      <Code>{`# Cursor: add to .cursor/mcp.json
{
  "mcpServers": {
    "fortress": {
      "command": "fortress",
      "args": ["serve", "--db", "/path/to/.fortress/jor-el.db"]
    }
  }
}`}</Code>
    </DocPage>
  )
}
