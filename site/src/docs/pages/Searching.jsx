import DocPage, { Code, Callout } from '../DocPage.jsx'

export default function Searching() {
  return (
    <DocPage slug="searching">
      <p>
        Cerebra provides two search modes: <strong>semantic search</strong> (vector similarity)
        and <strong>full-text search</strong> (FTS5 with porter stemming). Both are available via
        the CLI, MCP server, and web UI. Search works across both your <strong>codebase index</strong> and
        your <strong>agent conversation index</strong>, so you can find relevant code snippets and past
        agent session context in a single query.
      </p>

      <h2>Semantic search</h2>
      <p>
        Semantic search embeds your query and finds the most similar chunks by cosine similarity.
        This is great for natural language questions and works across code and agent conversations:
      </p>
      <Code>{`cerebra search "how does the payment processing pipeline work"
cerebra search "error handling in the API layer"
cerebra search "database migration strategy"`}</Code>

      <h2>Full-text search</h2>
      <p>
        FTS is used as a fallback when vector search returns no results, or when you're searching
        for exact identifiers:
      </p>
      <Code>{`cerebra search "handleWebhookEvent"
cerebra search "CREATE TABLE users"`}</Code>

      <h2>Search options</h2>
      <Code>{`# Limit results (default: 10)
cerebra search "auth middleware" --limit 5

# Search within a specific category
cerebra search "routing" --category go`}</Code>

      <Callout type="info">
        <strong>Tip:</strong> The MCP server and web UI use the same search pipeline.
        Both try vector search first, then fall back to FTS if no results are found.
        Results may include matches from both codebase documents and agent brain sessions.
      </Callout>

      <h2>How scoring works</h2>
      <p>
        Vector search results are ranked by <strong>cosine similarity</strong> between the query
        embedding and each chunk embedding. Scores closer to 1.0 mean higher relevance.
        Results include the file path, line numbers, category, and the matching content &mdash;
        whether the match comes from a code file or an agent conversation.
      </p>
    </DocPage>
  )
}
