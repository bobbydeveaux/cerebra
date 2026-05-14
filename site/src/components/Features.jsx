export default function Features() {
  const features = [
    {
      icon: '\u{1F50D}',
      color: 'var(--accent-dim)',
      title: 'Semantic Vector Search',
      desc: 'Find code by meaning, not just keywords. "How does authentication work?" returns the actual auth implementation, not every file containing the word "auth".',
    },
    {
      icon: '\u{1F9E0}',
      color: 'var(--green-dim)',
      title: 'Intelligent Chunking',
      desc: 'Code is chunked by function boundaries, not arbitrary line counts. Markdown by headings. Config files kept whole. Each chunk is a meaningful, searchable unit.',
    },
    {
      icon: '\u{26A1}',
      color: 'var(--amber-dim)',
      title: 'Incremental Scanning',
      desc: 'Content hashes and git SHAs ensure only changed files are re-embedded. Scan 20,000 files the first time. After that, only deltas. Sub-minute updates.',
    },
    {
      icon: '\u{1F517}',
      color: 'var(--purple-dim)',
      title: 'MCP Native',
      desc: "First-class Model Context Protocol support. Add one line to your AI tool's config and it instantly has semantic search across your entire codebase.",
    },
    {
      icon: '\u{1F512}',
      color: 'var(--red-dim)',
      title: 'Fully Local & Private',
      desc: 'Embeddings generated locally via Ollama. Database stored locally. No cloud. No API calls. No data leaves your laptop or your infrastructure. Ever.',
    },
    {
      icon: '\u{1F310}',
      color: 'var(--accent-dim)',
      title: 'Web UI + Chat',
      desc: 'Built-in wiki for browsing your indexed codebase. RAG-powered chat for asking questions. Search results with file links and line numbers. Zero JavaScript build step.',
    },
  ]

  return (
    <section className="section" id="features">
      <div className="section-label">Features</div>
      <h2 className="section-title">Everything you need.<br/>Nothing you don't.</h2>

      <div className="features-grid">
        {features.map((f) => (
          <div className="feature-card" key={f.title}>
            <div className="feature-icon" style={{ background: f.color }}>{f.icon}</div>
            <h3>{f.title}</h3>
            <p>{f.desc}</p>
          </div>
        ))}
      </div>
    </section>
  )
}
