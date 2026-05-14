export default function AgentMeeting() {
  return (
    <section className="meeting-section">
      <div className="meeting-content">
        <div className="section-label">Agent Meeting Mode</div>
        <h2 className="section-title">
          Agents that <span className="highlight">collaborate.</span>
        </h2>
        <p className="section-subtitle">
          Bring multiple agent brains into a structured discussion. Define an agenda,
          set required outcomes, and let your agents debate, refine, and produce
          permanent knowledge artefacts.
        </p>

        <div className="meeting-grid">
          <div className="meeting-visual">
            <div className="meeting-agent-list">
              <div className="meeting-agent">
                <span className="meeting-agent-dot" style={{ background: 'var(--violet)' }} />
                <span className="meeting-agent-name">Platform Agent</span>
                <span className="meeting-agent-role">Architecture</span>
              </div>
              <div className="meeting-agent">
                <span className="meeting-agent-dot" style={{ background: 'var(--coral)' }} />
                <span className="meeting-agent-name">Security Agent</span>
                <span className="meeting-agent-role">Threat Model</span>
              </div>
              <div className="meeting-agent">
                <span className="meeting-agent-dot" style={{ background: 'var(--gold)' }} />
                <span className="meeting-agent-name">Backend Agent</span>
                <span className="meeting-agent-role">Implementation</span>
              </div>
              <div className="meeting-agent">
                <span className="meeting-agent-dot" style={{ background: 'var(--green)' }} />
                <span className="meeting-agent-name">Docs Agent</span>
                <span className="meeting-agent-role">Documentation</span>
              </div>
            </div>
            <div className="terminal" style={{ marginTop: '1rem' }}>
              <div className="terminal-header">
                <span className="terminal-dot red" />
                <span className="terminal-dot yellow" />
                <span className="terminal-dot green" />
                <span className="terminal-title">meeting output</span>
              </div>
              <div className="terminal-body">
<span className="accent">## Decisions</span>{'\n'}
<span className="output">1. Use event-driven ingestion via webhooks</span>{'\n'}
<span className="output">2. JWT validation moves to gateway layer</span>{'\n'}
<span className="output">3. Rate limiting handled per-service</span>{'\n'}
{'\n'}
<span className="accent">## Actions</span>{'\n'}
<span className="output">- Platform: scaffold webhook handler</span>{'\n'}
<span className="output">- Security: audit token flow</span>{'\n'}
<span className="output">- Backend: implement rate limiter</span>
              </div>
            </div>
          </div>

          <div className="meeting-text">
            <h3>AI Architecture Council</h3>
            <p>
              Instead of asking one agent for an opinion, convene multiple specialised agents.
              Each brings its own context, challenges assumptions, and contributes domain expertise.
              The result is a permanent wiki artefact.
            </p>
            <p>
              Perfect for architecture decisions, incident reviews, security audits,
              sprint planning, and technical due diligence.
            </p>
            <h3 style={{ marginTop: '1.5rem' }}>Meeting Outputs</h3>
            <ul className="meeting-outputs">
              <li>Discussion summary with agent-by-agent contributions</li>
              <li>Decisions made with rationale</li>
              <li>Risks and trade-offs identified</li>
              <li>Action items assigned per agent</li>
              <li>Open questions for follow-up</li>
              <li>Wiki-ready Markdown artefact</li>
            </ul>
          </div>
        </div>
      </div>
    </section>
  )
}
