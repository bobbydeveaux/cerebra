import DocPage, { Code, Callout } from '../DocPage.jsx'

export default function Brains() {
  return (
    <DocPage slug="brains">
      <p>
        Every AI agent session becomes a <strong>brain</strong> &mdash; a tracked, indexed,
        searchable conversation stored in Cerebra's database. Brains are the core of Cerebra's
        cross-agent memory: context discovered in one session is automatically available to
        every future session.
      </p>

      <h2>How it works</h2>
      <p>
        Cerebra uses <code>fsnotify</code> to watch <code>~/.claude/projects/</code> for JSONL
        conversation files. When a new session file appears or an existing one is updated,
        Cerebra automatically parses, chunks, embeds, and indexes the conversation into the
        local SQLite vector database.
      </p>

      <h2>Starting the watcher</h2>
      <Code>{`# Watch for new and updated conversation files
cerebra brains watch --db-path .cerebra/jor-el.db`}</Code>
      <p>
        The watcher runs continuously in the background, picking up new sessions as they
        appear and incrementally indexing updates to active sessions.
      </p>

      <h2>What gets indexed</h2>
      <p>
        Cerebra indexes the full conversation content from each session:
      </p>
      <ul>
        <li><strong>User prompts</strong> &mdash; the questions and instructions you give the agent</li>
        <li><strong>Assistant responses</strong> &mdash; the agent's answers, explanations, and reasoning</li>
        <li><strong>Tool outputs</strong> &mdash; results from file reads, searches, command execution, and other tool calls</li>
      </ul>

      <h2>Summaries</h2>
      <p>
        Each session gets an auto-generated summary for token-efficient retrieval. Instead
        of injecting entire conversation histories into future prompts, Cerebra provides
        concise summaries that capture the key decisions, discoveries, and outcomes. The full
        conversation text remains searchable for when deeper context is needed.
      </p>

      <h2>Brain registry</h2>
      <p>
        Cerebra maintains a registry of all known brains with metadata:
      </p>
      <ul>
        <li><strong>Agent type</strong> &mdash; Claude Code, Cursor, Copilot, or other supported agents</li>
        <li><strong>Project</strong> &mdash; which project or repository the session was associated with</li>
        <li><strong>Status</strong> &mdash; whether the session is active, completed, or stale</li>
        <li><strong>Last seen</strong> &mdash; timestamp of the most recent activity</li>
        <li><strong>Summary</strong> &mdash; auto-generated summary of the session's content</li>
        <li><strong>Source path</strong> &mdash; the original JSONL file location</li>
      </ul>

      <h2>Listing brains</h2>
      <Code>{`# List all known brains
cerebra brains list`}</Code>

      <h2>MCP tools</h2>
      <p>
        Cerebra exposes brain data through its MCP server, making it available to any
        connected AI tool:
      </p>
      <ul>
        <li><code>search_brain</code> &mdash; semantic search across all indexed conversations</li>
        <li><code>list_brains</code> &mdash; list all known brains with metadata and summaries</li>
        <li><code>get_brain</code> &mdash; retrieve full details for a specific brain</li>
        <li><code>get_activity</code> &mdash; get recent activity across all brains</li>
      </ul>

      <Callout type="info">
        <strong>Agent-agnostic design.</strong> Cerebra uses a common schema that normalises
        conversations across Claude Code, Cursor, Copilot, and other AI tools. The brain
        registry and search work the same regardless of which agent produced the session.
      </Callout>

      <h2>Cross-agent discovery</h2>
      <p>
        The real power of brains is cross-agent discovery. When a new agent session starts,
        it can search the brain registry to find relevant context from previous sessions &mdash;
        even sessions run by different agents, on different projects, by different team members.
        This means:
      </p>
      <ul>
        <li>A new Claude Code session can discover what a previous session learned about a tricky bug</li>
        <li>Architecture decisions made in one session are available to all future sessions</li>
        <li>Knowledge doesn't get lost when a session ends &mdash; it persists in the brain registry</li>
        <li>Team members benefit from each other's agent interactions automatically</li>
      </ul>
    </DocPage>
  )
}
