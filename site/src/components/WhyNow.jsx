export default function WhyNow() {
  const cards = [
    {
      tag: 'Urgent',
      tagClass: 'urgent',
      title: 'AI Without Context = Technical Debt Factory',
      desc: "AI-generated code that doesn't follow your patterns becomes legacy code the moment it's merged. The longer you wait, the more AI-generated tech debt you accumulate.",
    },
    {
      tag: 'Opportunity',
      tagClass: 'opportunity',
      title: 'MCP Is the Standard',
      desc: 'Model Context Protocol is becoming the universal way AI tools consume context. Fortress is MCP-native from day one. Build your infrastructure on the standard, not on proprietary integrations.',
    },
    {
      tag: 'Risk',
      tagClass: 'risk',
      title: 'Your Engineers Are Already Using AI',
      desc: "They're using Copilot, Claude, ChatGPT \u2014 with or without your blessing. The question isn't whether they use AI. It's whether you give them the context layer to use it well.",
    },
    {
      tag: 'Open Source',
      tagClass: 'movement',
      title: 'No Vendor Lock-In',
      desc: 'Fortress runs locally, uses open standards (SQLite, Ollama, MCP), and the entire codebase is open source. Inspect it, fork it, extend it. Your knowledge base belongs to you.',
    },
  ]

  return (
    <section className="section" id="why-now">
      <div className="section-label">Why Now</div>
      <h2 className="section-title">The AI adoption window<br/>is closing fast.</h2>
      <p className="section-subtitle">
        Every month you delay, your competitors' AI tools get smarter about their codebases while yours stays blind.
      </p>

      <div className="why-grid">
        {cards.map((c) => (
          <div className="why-card" key={c.title}>
            <div className={`why-tag ${c.tagClass}`}>{c.tag}</div>
            <h3>{c.title}</h3>
            <p>{c.desc}</p>
          </div>
        ))}
      </div>
    </section>
  )
}
