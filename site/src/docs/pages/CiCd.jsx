import DocPage, { Code, Callout } from '../DocPage.jsx'

export default function CiCd() {
  return (
    <DocPage slug="ci-cd">
      <p>
        Keep your Cerebra index fresh automatically. Every time code is pushed, a GitHub Action
        re-scans the changed repo and uploads the updated database to cloud storage where your
        team can pull it.
      </p>

      <h2>How it works</h2>
      <ol>
        <li>Developer pushes code to a repo</li>
        <li>GitHub Action triggers, downloads the current Cerebra DB from cloud storage</li>
        <li>Cerebra incrementally re-scans only the changed repo</li>
        <li>Updated DB is uploaded back to cloud storage</li>
        <li>Engineers pull the latest DB &mdash; or it syncs automatically</li>
      </ol>

      <h2>GitHub Action setup</h2>
      <p>
        Add this workflow to any repo you want to keep indexed. Create
        <code> .github/workflows/cerebra-index.yml</code>:
      </p>
      <Code>{`name: Cerebra Reindex

on:
  push:
    branches: [main]
  workflow_dispatch:

jobs:
  reindex:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0   # Full history for git indexing

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.22'

      - name: Install Cerebra
        run: go install github.com/bobbydeveaux/cerebra@latest

      - name: Download current DB
        run: |
          # Download from your cloud storage
          # GCS example:
          gsutil cp gs://\${{ vars.CEREBRA_BUCKET }}/jor-el.db .cerebra/jor-el.db || true
          # S3 example:
          # aws s3 cp s3://\${{ vars.CEREBRA_BUCKET }}/jor-el.db .cerebra/jor-el.db || true

      - name: Scan
        env:
          OPENAI_API_KEY: \${{ secrets.OPENAI_API_KEY }}
        run: cerebra scan .

      - name: Upload updated DB
        run: |
          # GCS example:
          gsutil cp .cerebra/jor-el.db gs://\${{ vars.CEREBRA_BUCKET }}/jor-el.db
          # S3 example:
          # aws s3 cp .cerebra/jor-el.db s3://\${{ vars.CEREBRA_BUCKET }}/jor-el.db`}</Code>

      <Callout type="info">
        <strong>Incremental by default.</strong> Since Cerebra tracks content hashes, the CI scan
        only re-embeds files that changed in the push. A typical re-index on a large repo takes
        seconds, not minutes.
      </Callout>

      <h2>Required secrets and variables</h2>
      <p>Set these in your GitHub repo settings:</p>
      <ul>
        <li><code>OPENAI_API_KEY</code> (secret) &mdash; if using OpenAI embeddings</li>
        <li><code>CEREBRA_BUCKET</code> (variable) &mdash; your GCS/S3 bucket path</li>
        <li>Cloud provider credentials (GCP Workload Identity or AWS role)</li>
      </ul>

      <h2>Multi-repo setup</h2>
      <p>
        For organisations with many repos, create a <strong>central indexing repo</strong> that
        checks out and scans all repos into a single database:
      </p>
      <Code>{`name: Cerebra Central Index

on:
  schedule:
    - cron: '0 2 * * *'    # Nightly at 2am
  workflow_dispatch:

jobs:
  reindex:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout all repos
        run: |
          gh repo list my-org --limit 200 --json name -q '.[].name' | while read repo; do
            gh repo clone "my-org/\$repo" "repos/\$repo" -- --depth 1 || true
          done
        env:
          GH_TOKEN: \${{ secrets.GH_PAT }}

      - name: Install Cerebra
        run: go install github.com/bobbydeveaux/cerebra@latest

      - name: Download current DB
        run: gsutil cp gs://\${{ vars.CEREBRA_BUCKET }}/jor-el.db .cerebra/jor-el.db || true

      - name: Scan all repos
        env:
          OPENAI_API_KEY: \${{ secrets.OPENAI_API_KEY }}
        run: cerebra scan ./repos

      - name: Upload updated DB
        run: gsutil cp .cerebra/jor-el.db gs://\${{ vars.CEREBRA_BUCKET }}/jor-el.db`}</Code>

      <Callout type="warning">
        <strong>Database locking:</strong> If multiple repos push simultaneously, they could
        conflict writing to the same DB file in cloud storage. For high-traffic orgs,
        use the central indexing approach with a scheduled job instead.
      </Callout>

      <h2>Local sync</h2>
      <p>
        Engineers can pull the latest database to their machine:
      </p>
      <Code>{`# GCS
gsutil cp gs://my-bucket/cerebra/jor-el.db .cerebra/jor-el.db

# S3
aws s3 cp s3://my-bucket/cerebra/jor-el.db .cerebra/jor-el.db

# Or use Cerebra built-in cloud sync
cerebra scan --cloud gs://my-bucket/cerebra/`}</Code>
    </DocPage>
  )
}
