export default function Solution() {
  return (
    <section className="section" id="solution">
      <div className="section-label">The Solution</div>
      <h2 className="section-title">Give your AI the one thing<br/>it's missing: <span className="text-accent">your code.</span></h2>
      <p className="section-subtitle">
        Fortress scans your entire codebase, generates vector embeddings, and serves them over MCP &mdash; so every AI tool in your organisation actually understands what it's working with.
      </p>

      <div className="solution-grid">
        <div className="solution-card">
          <div className="solution-number">01</div>
          <h3>Scan Everything</h3>
          <p>Point Fortress at your repos. It walks every file, detects languages and categories, chunks intelligently by function boundaries, and generates embeddings locally using Ollama. No code leaves your machine.</p>
        </div>
        <div className="solution-card">
          <div className="solution-number">02</div>
          <h3>Build the Knowledge Base</h3>
          <p>All embeddings land in a local SQLite vector database. Full-text search. Semantic search. Git history. Auto-generated documentation. Everything indexed, everything queryable, everything private.</p>
        </div>
        <div className="solution-card">
          <div className="solution-number">03</div>
          <h3>Serve to Every AI Tool</h3>
          <p>Fortress exposes an MCP server that any compatible AI tool can connect to. Claude Code, Cursor, Windsurf, custom agents &mdash; they all get instant, semantic access to your entire codebase.</p>
        </div>
      </div>
    </section>
  )
}
