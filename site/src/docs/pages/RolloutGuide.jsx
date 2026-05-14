import DocPage, { Code, Callout } from '../DocPage.jsx'

export default function RolloutGuide() {
  return (
    <DocPage slug="rollout-guide">
      <p>
        This guide is for <strong>platform teams</strong> who want to roll Cerebra out to their
        entire engineering organisation. The goal: every developer's AI tools have full context
        about every repo, automatically kept up to date.
      </p>

      <Callout type="info">
        <strong>Cross-agent memory sharing included.</strong> Rolling out Cerebra doesn't just give
        your team codebase knowledge &mdash; it also enables cross-agent memory sharing. Every AI
        agent session is indexed as a "brain", meaning context discovered by one agent is
        automatically available to others. New engineers benefit from the institutional knowledge
        captured across all previous agent sessions, not just the codebase itself.
      </Callout>

      <h2>Architecture overview</h2>
      <ol>
        <li><strong>Central index job</strong> &mdash; a scheduled GitHub Action scans all repos nightly</li>
        <li><strong>Cloud storage</strong> &mdash; the database is uploaded to GCS/S3 after each scan</li>
        <li><strong>Per-repo hooks</strong> &mdash; individual repos trigger incremental reindexes on push</li>
        <li><strong>Developer machines</strong> &mdash; engineers download the DB and register the MCP server</li>
      </ol>

      <h2>Step 1: Create the infrastructure</h2>
      <Code>{`# Create a GCS bucket (or S3)
gsutil mb gs://mycompany-cerebra

# Create a service account for CI
gcloud iam service-accounts create cerebra-ci \\
  --display-name "Cerebra CI"

# Grant storage access
gsutil iam ch \\
  serviceAccount:cerebra-ci@myproject.iam.gserviceaccount.com:objectAdmin \\
  gs://mycompany-cerebra`}</Code>

      <h2>Step 2: Central nightly index</h2>
      <p>
        Create a dedicated repo (e.g. <code>myorg/cerebra-index</code>) with the
        central indexing workflow from the <a href="/docs/ci-cd">CI/CD guide</a>.
        This scans all repos in your GitHub org nightly.
      </p>

      <h2>Step 3: Per-repo incremental updates</h2>
      <p>
        For repos that change frequently, add the per-repo GitHub Action so pushes to
        main trigger an immediate reindex. See the <a href="/docs/ci-cd">CI/CD guide</a> for the workflow file.
      </p>

      <h2>Step 4: Developer onboarding</h2>
      <p>Add these steps to your developer onboarding guide:</p>
      <Code>{`# 1. Install Cerebra
go install github.com/bobbydeveaux/cerebra@latest

# 2. Download the shared database
gsutil cp gs://mycompany-cerebra/jor-el.db ~/.cerebra/jor-el.db

# 3. Add to Claude Code
claude mcp add cerebra -- cerebra serve --db ~/.cerebra/jor-el.db

# 4. (Optional) Add a cron to keep it fresh
# Add to crontab:
# 0 9 * * * gsutil cp gs://mycompany-cerebra/jor-el.db ~/.cerebra/jor-el.db`}</Code>

      <Callout type="success">
        <strong>That's it.</strong> Every AI interaction &mdash; Claude Code, Cursor, Copilot via MCP &mdash;
        now has full context about your entire codebase and cross-agent memory. New engineers get it on day one.
      </Callout>

      <h2>Measuring impact</h2>
      <p>Track these metrics to demonstrate ROI to leadership:</p>
      <ul>
        <li><strong>Onboarding time</strong> &mdash; how fast new engineers make their first meaningful PR</li>
        <li><strong>AI tool adoption</strong> &mdash; % of engineers with Cerebra MCP configured</li>
        <li><strong>Cross-team PRs</strong> &mdash; engineers contributing to repos outside their team</li>
        <li><strong>Question resolution</strong> &mdash; "how does X work?" answered by AI vs. Slack</li>
        <li><strong>Cross-agent context reuse</strong> &mdash; how often agents find relevant brains from previous sessions</li>
      </ul>

      <h2>Security considerations</h2>
      <ul>
        <li>The database contains code snippets and agent conversations &mdash; apply the same access controls as your source code</li>
        <li>Use IAM roles for cloud storage access, not shared keys</li>
        <li>Cerebra runs 100% locally on developer machines &mdash; no code leaves the network</li>
        <li>Embedding API calls (OpenAI) send code snippets to the provider. Use Ollama for fully local operation.</li>
      </ul>
    </DocPage>
  )
}
