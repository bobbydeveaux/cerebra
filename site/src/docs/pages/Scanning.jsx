import DocPage, { Code, Callout, Table } from '../DocPage.jsx'

export default function Scanning() {
  return (
    <DocPage slug="scanning">
      <p>
        The scanner is the heart of Fortress. It recursively walks your codebase, detects languages,
        chunks files intelligently, generates embeddings, and stores everything in the local SQLite database.
      </p>

      <h2>Basic usage</h2>
      <Code>{`# Scan a single repo
fortress scan ./my-project

# Scan multiple repos at once
fortress scan ./repos/api ./repos/frontend ./repos/infra

# Scan a parent directory (discovers all repos inside)
fortress scan ./all-repos`}</Code>

      <h2>How chunking works</h2>
      <p>Fortress chunks files differently based on their type:</p>
      <Table
        headers={['File type', 'Strategy', 'Chunk boundaries']}
        rows={[
          ['Code (Go, JS, Python, etc.)', 'Function-level', 'Each function/method becomes a chunk'],
          ['Markdown', 'Heading-level', 'Split on ## headings'],
          ['Config (YAML, JSON, TOML)', 'Whole file', 'Entire file as one chunk'],
          ['Other', 'Fixed-size', 'Split at chunk_size with overlap'],
        ]}
      />

      <h2>Incremental scanning</h2>
      <p>
        Fortress tracks a <code>content_hash</code> for every file and <code>last_commit_sha</code> per repo.
        When you re-run <code>fortress scan</code>, only changed files are re-embedded. This makes
        subsequent scans dramatically faster.
      </p>

      <h2>Git history indexing</h2>
      <p>
        Fortress indexes git commit messages as searchable documents. This means your AI tools can
        answer questions like "when was the auth module last changed?" or "what was the motivation for
        the database migration?"
      </p>

      <h2>Ignore patterns</h2>
      <p>
        Control which files are scanned via <code>fortress.yaml</code>. By default, Fortress skips
        common non-code files:
      </p>
      <Code>{`# fortress.yaml
ignore:
  - .git
  - node_modules
  - vendor
  - "*.bin"
  - "*.lock"
  - dist
  - build
  - "*.min.js"
  - "*.png"
  - "*.jpg"`}</Code>

      <Callout type="info">
        <strong>Large monorepos?</strong> Add ignore patterns for data files, generated code,
        and binary assets to reduce scan time and database size significantly.
      </Callout>

      <h2>Performance</h2>
      <p>Scan speed depends on your embedding provider and codebase size:</p>
      <Table
        headers={['Codebase', 'Files', 'Chunks', 'Time (Ollama)', 'DB size']}
        rows={[
          ['Small project', '~500', '~3,000', '~30s', '~50 MB'],
          ['Medium project', '~2,000', '~12,000', '~3 min', '~200 MB'],
          ['Large monorepo', '~20,000', '~120,000', '~20 min', '~1.4 GB'],
        ]}
      />
    </DocPage>
  )
}
