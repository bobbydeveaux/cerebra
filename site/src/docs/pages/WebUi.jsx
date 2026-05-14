import DocPage, { Code, Callout } from '../DocPage.jsx'

export default function WebUi() {
  return (
    <DocPage slug="web-ui">
      <p>
        Fortress includes a built-in web UI for browsing your indexed codebase and chatting with it
        using RAG (Retrieval-Augmented Generation). No JavaScript build step required &mdash; it's
        pure <strong>htmx</strong>.
      </p>

      <h2>Start the web UI</h2>
      <Code>{`fortress serve --ui`}</Code>
      <p>Open <code>http://localhost:8080</code> in your browser.</p>

      <h2>Wiki browser</h2>
      <p>
        The wiki gives you a navigable view of your entire indexed codebase, organised by category
        (language/file type). You can:
      </p>
      <ul>
        <li>Browse all categories (Go, JavaScript, Python, Markdown, etc.)</li>
        <li>Drill into individual files and see their chunks</li>
        <li>View stats: repo count, file count, chunk count</li>
      </ul>

      <h2>Search</h2>
      <p>
        The search page provides the same semantic + FTS search available in the CLI,
        with results showing file paths, line numbers, and relevance scores.
      </p>

      <h2>RAG chat</h2>
      <p>
        The chat interface lets you ask questions about your codebase in natural language.
        Fortress retrieves relevant code chunks and sends them as context to an LLM for
        grounded answers with citations.
      </p>

      <Callout type="info">
        <strong>Conversation history:</strong> Follow-up questions have context from previous
        messages in the same session. Ask "how does auth work?" then "what does the middleware look like?"
        and Fortress understands the context.
      </Callout>

      <h2>Chat LLM providers</h2>
      <p>
        The RAG chat supports multiple LLM providers. Configure which one to use
        in <code>fortress.yaml</code>:
      </p>
      <Code>{`# fortress.yaml
chat_llm: openai   # Options: ollama, openai, claude, minimax`}</Code>

      <h2>Configuration</h2>
      <Code>{`# fortress.yaml
ui_port: 8080
ui_bind: 127.0.0.1   # Use 0.0.0.0 to expose to network`}</Code>
    </DocPage>
  )
}
