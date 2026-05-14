import DocPage, { Code, Callout, Table } from '../DocPage.jsx'

export default function CloudStorage() {
  return (
    <DocPage slug="cloud-storage">
      <p>
        Fortress treats the SQLite database as a portable artifact. Upload it to cloud storage
        and share it across your entire engineering org. Every developer gets the same
        knowledge base without running their own scan.
      </p>

      <h2>Supported providers</h2>
      <Table
        headers={['Provider', 'URI format', 'Auth']}
        rows={[
          ['Google Cloud Storage', <code>gs://bucket/path/</code>, 'Application Default Credentials or service account'],
          ['Amazon S3', <code>s3://bucket/path/</code>, 'AWS credentials or IAM role'],
        ]}
      />

      <h2>Configuration</h2>
      <Code>{`# fortress.yaml
cloud_storage: "gs://my-company-fortress/prod/"`}</Code>

      <h2>Upload after scanning</h2>
      <Code>{`# Scan and upload
fortress scan ./repos
gsutil cp .fortress/jor-el.db gs://my-company-fortress/prod/jor-el.db`}</Code>

      <h2>Download for local use</h2>
      <Code>{`# Download the shared database
gsutil cp gs://my-company-fortress/prod/jor-el.db .fortress/jor-el.db

# Now use it locally
fortress search "how does deployment work"
fortress serve    # MCP server with full org context`}</Code>

      <Callout type="info">
        <strong>The database is self-contained.</strong> A single <code>jor-el.db</code> file
        contains all embeddings, metadata, and full-text search indexes. Copy it anywhere
        and it just works.
      </Callout>

      <h2>Recommended setup for teams</h2>
      <ol>
        <li>Create a cloud storage bucket for your org</li>
        <li>Set up the <a href="/docs/ci-cd">CI/CD GitHub Action</a> to scan and upload on every push</li>
        <li>Have developers download the DB or set up a local sync script</li>
        <li>Point all AI tools (Claude Code, Cursor) at the shared database</li>
      </ol>
    </DocPage>
  )
}
