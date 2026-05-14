export default function Solution() {
  return (
    <section className="section" id="solution">
      <div className="section-label">The Solution</div>
      <h2 className="section-title">
        Give your agents <span className="highlight">persistent memory.</span>
      </h2>
      <p className="section-subtitle">
        Cerebra watches your AI conversations in real time, indexes every interaction,
        generates concise summaries, and makes it all searchable &mdash; so every agent
        session starts with the full picture.
      </p>

      <div className="solution-grid">
        <div className="solution-card">
          <div className="solution-number">01</div>
          <h3>Watch Conversations</h3>
          <p>
            Cerebra monitors your agent session directories using native filesystem events.
            New conversation files are detected instantly &mdash; no polling, no manual triggers.
            Works with Claude Code out of the box, extensible to any agent.
          </p>
        </div>
        <div className="solution-card">
          <div className="solution-number">02</div>
          <h3>Index Everything</h3>
          <p>
            Every conversation is parsed into a common schema, chunked semantically, and
            embedded into a local SQLite vector database. Incremental by default &mdash;
            only new content is processed.
          </p>
        </div>
        <div className="solution-card">
          <div className="solution-number">03</div>
          <h3>Summarise &amp; Distil</h3>
          <p>
            Each session gets a concise, evolving summary. Key decisions, patterns discovered,
            problems solved &mdash; distilled into compact context that doesn't bloat prompts
            or waste tokens.
          </p>
        </div>
        <div className="solution-card">
          <div className="solution-number">04</div>
          <h3>Connect Every Agent</h3>
          <p>
            Expose the full knowledge base via MCP. Any agent can search across all brains,
            discover what other agents have learned, and start with context instead of from scratch.
          </p>
        </div>
      </div>
    </section>
  )
}
