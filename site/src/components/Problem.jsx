export default function Problem() {
  return (
    <section className="section" id="problem">
      <div className="section-label">The Problem</div>
      <h2 className="section-title">AI without context is just<br/>expensive autocomplete.</h2>
      <p className="section-subtitle">
        Your company just gave every developer an AI licence. Congratulations &mdash; you've spent six figures on a tool that doesn't know the difference between your auth service and your billing API.
      </p>

      <div className="problem-grid">
        <div className="problem-card">
          <div className="problem-icon">&#x1f648;</div>
          <h3>Agents Working Blind</h3>
          <p>AI coding assistants generate plausible-looking code that ignores your conventions, duplicates existing utilities, and misunderstands your domain. Engineers spend more time fixing AI output than they saved.</p>
        </div>
        <div className="problem-card">
          <div className="problem-icon">&#x1f4b8;</div>
          <h3>Wasted AI Spend</h3>
          <p>You're paying per-token for AI models to guess what your codebase looks like. Without embeddings, every prompt starts from zero. No memory. No learning. Just expensive hallucination.</p>
        </div>
        <div className="problem-card">
          <div className="problem-icon">&#x1f622;</div>
          <h3>Developer Frustration</h3>
          <p>"AI doesn't work for our codebase" is the #1 complaint. It's not that AI doesn't work &mdash; it's that nobody gave it the context it needs. Your developers blame the tool when the problem is the setup.</p>
        </div>
      </div>

      <div className="cost-banner">
        <div className="cost-banner-icon">&#x26a0;&#xfe0f;</div>
        <div>
          <h3>The uncomfortable truth</h3>
          <p>If your engineers are using AI tools without a codebase-aware context layer, they're not doing AI-assisted development. They're doing <strong>AI-confused development</strong>. And it's costing you real engineering hours every single day.</p>
        </div>
      </div>
    </section>
  )
}
