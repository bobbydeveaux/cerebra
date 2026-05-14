export default function Architecture() {
  return (
    <section className="arch-section" id="architecture">
      <div className="arch-content">
        <div className="section-label">Architecture</div>
        <h2 className="section-title">
          From conversation to <span className="highlight">collective memory.</span>
        </h2>
        <p className="section-subtitle">
          Cerebra builds on top of a proven codebase indexing pipeline and extends it with
          agent-aware ingestion, summarisation, and cross-agent discovery.
        </p>

        <div className="arch-flow">
          <div className="arch-node">
            <div className="arch-node-icon">&#x1f916;</div>
            <h4>Agent Session</h4>
            <p>JSONL conversation</p>
          </div>
          <div className="arch-arrow">&#x2192;</div>
          <div className="arch-node">
            <div className="arch-node-icon">&#x1f441;&#xfe0f;</div>
            <h4>Watcher</h4>
            <p>fsnotify events</p>
          </div>
          <div className="arch-arrow">&#x2192;</div>
          <div className="arch-node">
            <div className="arch-node-icon">&#x2699;&#xfe0f;</div>
            <h4>Parser</h4>
            <p>Common schema</p>
          </div>
          <div className="arch-arrow">&#x2192;</div>
          <div className="arch-node">
            <div className="arch-node-icon">&#x1f9e9;</div>
            <h4>Embedder</h4>
            <p>Ollama / OpenAI</p>
          </div>
          <div className="arch-arrow">&#x2192;</div>
          <div className="arch-node">
            <div className="arch-node-icon">&#x1f4be;</div>
            <h4>Jor-El DB</h4>
            <p>SQLite + vec</p>
          </div>
        </div>

        <div className="arch-detail-grid">
          <div className="arch-detail">
            <h3>Codebase + Conversations</h3>
            <p>
              Cerebra inherits the full scan pipeline from Fortress &mdash; code files, git history,
              Confluence pages. On top of that, it adds conversation-aware ingestion that
              parses agent session files into the same vector store.
            </p>
            <div className="terminal-inline">
              <span className="prompt">$ </span>cerebra scan ~/code/my-org<br/>
              <span className="prompt">$ </span>cerebra brains watch
            </div>
          </div>
          <div className="arch-detail">
            <h3>Three Runtime Modes</h3>
            <p>
              All modes share a single SQLite database, so everything stays in sync.
            </p>
            <ul className="arch-checklist">
              <li><code>cerebra scan</code> &mdash; ingest codebases and documentation</li>
              <li><code>cerebra brains watch</code> &mdash; monitor agent conversations</li>
              <li><code>cerebra serve</code> &mdash; MCP server for agent integration</li>
              <li><code>cerebra serve --ui</code> &mdash; web dashboard, wiki, and chat</li>
            </ul>
          </div>
        </div>
      </div>
    </section>
  )
}
