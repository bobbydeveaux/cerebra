import DocPage, { Code, Callout } from '../DocPage.jsx'

export default function Installation() {
  return (
    <DocPage slug="installation">
      <h2>Prerequisites</h2>
      <ul>
        <li><strong>Go 1.22+</strong> &mdash; Cerebra is built with Go</li>
        <li><strong>Ollama</strong> (recommended) &mdash; for local embeddings with <code>nomic-embed-text</code></li>
        <li>Or an <strong>OpenAI API key</strong> for cloud-based embeddings</li>
      </ul>

      <h2>Install from source</h2>
      <Code>{`# Clone the repo
git clone https://github.com/bobbydeveaux/cerebra.git
cd cerebra

# Build the binary
go build -o cerebra .

# Move to your PATH
sudo mv cerebra /usr/local/bin/

# Verify
cerebra --help`}</Code>

      <h2>Install via Go</h2>
      <Code>{`go install github.com/bobbydeveaux/cerebra@latest`}</Code>

      <h2>Set up Ollama (recommended)</h2>
      <p>
        Cerebra uses Ollama by default for local embeddings. Install Ollama and pull the embedding model:
      </p>
      <Code>{`# Install Ollama (macOS)
brew install ollama

# Start Ollama
ollama serve

# Pull the embedding model
ollama pull nomic-embed-text`}</Code>

      <Callout type="info">
        <strong>Using OpenAI instead?</strong> Set your API key in <code>cerebra.yaml</code> or
        as an environment variable: <code>export OPENAI_API_KEY=sk-...</code>, then
        set <code>embedder: openai</code> in your config.
      </Callout>

      <h2>Verify installation</h2>
      <Code>{`$ cerebra --help
Cerebra — codebase knowledge + agent memory for AI tools

Usage:
  cerebra [command]

Available Commands:
  scan     Scan and index a codebase
  search   Search the knowledge base
  serve    Start MCP or web UI server
  brains   Manage the agent brain registry
  stats    Show database statistics
  watch    Watch for changes and re-scan
  forget   Remove a repo from the index`}</Code>

      <p>You're ready to go. Head to <a href="/docs/quickstart">Quick Start</a> to index your first codebase.</p>
    </DocPage>
  )
}
