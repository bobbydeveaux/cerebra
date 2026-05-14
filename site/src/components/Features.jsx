export default function Features() {
  return (
    <section className="section" id="features">
      <div className="section-label">Features</div>
      <h2 className="section-title">
        Memory infrastructure for <span className="text-violet">AI agents.</span>
      </h2>
      <p className="section-subtitle">
        Everything you need to give your agents persistent, shared, searchable memory.
        Built for engineers who ship with AI every day.
      </p>

      <div className="features-grid">
        <div className="feature-card">
          <div className="feature-icon" style={{ background: 'var(--violet-dim)' }}>&#x1f52e;</div>
          <h3>Brain Registry</h3>
          <p>
            Every agent session is tracked as a "brain" with metadata &mdash; project, agent type,
            last activity, summary, linked repos. See your entire agent fleet at a glance.
          </p>
        </div>
        <div className="feature-card">
          <div className="feature-icon" style={{ background: 'var(--coral-dim)' }}>&#x26a1;</div>
          <h3>Real-time Watcher</h3>
          <p>
            Native fsnotify watches conversation directories for changes. New content is indexed
            within seconds. No cron jobs, no manual re-scans &mdash; always up to date.
          </p>
        </div>
        <div className="feature-card">
          <div className="feature-icon" style={{ background: 'var(--gold-dim)' }}>&#x1f50d;</div>
          <h3>Semantic Search</h3>
          <p>
            Ask "how did we handle rate limiting?" and get results from agent conversations,
            codebase scans, and documentation. Vector similarity meets full-text search.
          </p>
        </div>
        <div className="feature-card">
          <div className="feature-icon" style={{ background: 'var(--rose-dim)' }}>&#x1f4dd;</div>
          <h3>Auto Summaries</h3>
          <p>
            Every session gets a continuously-updated summary. Decisions, patterns, insights
            distilled into compact context. Agents get the gist without reading thousands of tokens.
          </p>
        </div>
        <div className="feature-card">
          <div className="feature-icon" style={{ background: 'var(--green-dim)' }}>&#x1f517;</div>
          <h3>MCP Native</h3>
          <p>
            First-class Model Context Protocol server. One config line to connect Claude Code,
            Cursor, or any MCP-compatible tool. search_brain, list_brains, get_activity built in.
          </p>
        </div>
        <div className="feature-card">
          <div className="feature-icon" style={{ background: 'var(--violet-dim)' }}>&#x1f310;</div>
          <h3>Web UI + Chat</h3>
          <p>
            Built-in web interface with brain dashboard, wiki, and RAG-powered chat.
            Search across all agent knowledge visually. htmx-powered &mdash; no JS build step.
          </p>
        </div>
      </div>
    </section>
  )
}
