export default function Problem() {
  return (
    <section className="section" id="problem">
      <div className="section-label">The Problem</div>
      <h2 className="section-title">
        Every AI session starts from <span className="text-coral">zero.</span>
      </h2>
      <p className="section-subtitle">
        Your agents have no memory. Every conversation, every decision, every hard-won
        insight vanishes the moment the session ends. The next agent starts completely blind.
      </p>

      <div className="problem-grid">
        <div className="problem-card">
          <div className="problem-icon">&#x1f9e0;</div>
          <h3>Amnesia by Design</h3>
          <p>
            Claude, Copilot, Cursor &mdash; none of them remember previous sessions.
            Your agent solves the same problem three times in a week because it has
            no memory of solving it before.
          </p>
        </div>
        <div className="problem-card">
          <div className="problem-icon">&#x1f50d;</div>
          <h3>Lost Context, Wasted Tokens</h3>
          <p>
            Every session burns tokens re-discovering what a previous session already knew.
            Architecture decisions, API patterns, debugging insights &mdash; all evaporate
            between conversations.
          </p>
        </div>
        <div className="problem-card">
          <div className="problem-icon">&#x1f6ab;</div>
          <h3>Isolated Agents, Duplicated Work</h3>
          <p>
            Running multiple agents across repos? They can't share what they've learned.
            Agent A refactors auth while Agent B re-implements the old pattern in another service.
          </p>
        </div>
      </div>

      <div className="cost-banner">
        <div className="cost-banner-icon">&#x26a0;&#xfe0f;</div>
        <div>
          <h3>The compounding cost of forgetting</h3>
          <p>
            Without persistent memory, your AI agents don't get smarter over time. They stay
            perpetually junior &mdash; making the same mistakes, asking the same questions,
            missing the same context. Every day.
          </p>
        </div>
      </div>
    </section>
  )
}
