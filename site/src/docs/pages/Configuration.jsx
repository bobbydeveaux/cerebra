import DocPage, { Code, Table, Callout } from '../DocPage.jsx'

export default function Configuration() {
  return (
    <DocPage slug="configuration">
      <p>
        Fortress is configured via a <code>fortress.yaml</code> file. It looks in the current
        directory first, then falls back to <code>~/fortress.yaml</code>.
      </p>

      <h2>Full example</h2>
      <Code>{`# fortress.yaml

# Embedding provider: "ollama" or "openai"
embedder: ollama

ollama:
  url: http://localhost:11434
  embed_model: nomic-embed-text
  chat_model: llama3.2

openai:
  api_key: ""                      # or set OPENAI_API_KEY env var
  embed_model: text-embedding-3-small
  chat_model: gpt-4o

# Chat LLM for RAG: ollama, openai, claude, minimax
chat_llm: openai

claude:
  api_key: ""                      # or set ANTHROPIC_API_KEY env var
  model: claude-sonnet-4-6

minimax:
  api_key: ""                      # or set MINIMAX_API_KEY env var
  model: MiniMax-M2.7-highspeed

# Files/directories to skip during scanning
ignore:
  - .git
  - node_modules
  - vendor
  - "*.bin"
  - "*.lock"
  - dist
  - build

# Chunking settings
chunk_size: 512
chunk_overlap: 64

# Database
db_path: .fortress/jor-el.db
docs_path: .fortress/docs/

# Cloud storage for DB sync (optional)
cloud_storage: ""   # e.g. gs://my-bucket/fortress/

# Web UI
ui_port: 8080
ui_bind: 127.0.0.1

# Embedding concurrency
embed_workers: 4
embed_batch_size: 32`}</Code>

      <h2>Configuration reference</h2>
      <Table
        headers={['Key', 'Default', 'Description']}
        rows={[
          [<code>embedder</code>, '"ollama"', 'Embedding provider: ollama or openai'],
          [<code>chat_llm</code>, '"ollama"', 'Chat LLM: ollama, openai, claude, or minimax'],
          [<code>chunk_size</code>, '512', 'Target chunk size in tokens'],
          [<code>chunk_overlap</code>, '64', 'Overlap between adjacent chunks'],
          [<code>db_path</code>, '.fortress/jor-el.db', 'SQLite database path'],
          [<code>docs_path</code>, '.fortress/docs/', 'Generated markdown docs path'],
          [<code>cloud_storage</code>, '""', 'GCS or S3 URI for DB sync'],
          [<code>ui_port</code>, '8080', 'Web UI port'],
          [<code>ui_bind</code>, '"127.0.0.1"', 'Web UI bind address'],
          [<code>embed_workers</code>, '2', 'Concurrent embedding workers'],
          [<code>embed_batch_size</code>, '32', 'Texts per embedding batch'],
        ]}
      />

      <h2>Environment variables</h2>
      <p>API keys can be set as environment variables instead of in the config file:</p>
      <Table
        headers={['Variable', 'Description']}
        rows={[
          [<code>OPENAI_API_KEY</code>, 'OpenAI API key (embeddings and chat)'],
          [<code>ANTHROPIC_API_KEY</code>, 'Anthropic API key (Claude chat)'],
          [<code>MINIMAX_API_KEY</code>, 'MiniMax API key (chat)'],
        ]}
      />

      <Callout type="warning">
        <strong>Never commit API keys.</strong> Use environment variables or keep
        your <code>fortress.yaml</code> in your home directory (<code>~/fortress.yaml</code>)
        outside of any git repository.
      </Callout>
    </DocPage>
  )
}
