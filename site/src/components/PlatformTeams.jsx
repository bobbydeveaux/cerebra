export default function PlatformTeams() {
  return (
    <section className="platform-section" id="platform-teams">
      <div className="platform-content">
        <div className="section-label">For Platform Teams</div>
        <h2 className="section-title">Stop letting engineers<br/>use AI in the dark.</h2>
        <p className="section-subtitle">
          Individual developers running Copilot blind is a problem. At scale, it's an organisational failure. Platform teams need to own the AI context layer.
        </p>

        <div className="platform-grid">
          <div className="platform-text">
            <h3>The Bare Minimum</h3>
            <p>
              Every engineering team should have their codebase embedded in a local vector database. This is not advanced AI tooling. This is <strong>table stakes</strong>. Without it, every AI interaction starts from zero context and produces unreliable output.
            </p>
            <p>
              Fortress makes this trivial: one command to scan, one config line to connect. Any developer can set it up in 30 seconds.
            </p>
            <div className="terminal-inline">
              <span className="prompt">$</span> <span className="cmd">fortress scan .</span><br/>
              <span className="prompt">$</span> <span className="cmd">fortress serve</span>
            </div>
          </div>

          <div className="platform-text">
            <h3>The Real Play: Centralised Vector DB</h3>
            <p>
              The advanced move is a platform-managed Fortress instance. One vector DB covering every repo, every service, every piece of infrastructure. Served via MCP to every engineer's AI tool &mdash; mandatory, not optional.
            </p>
            <ul className="platform-checklist">
              <li>Central DB covering the entire org's codebase</li>
              <li>MCP endpoint that every AI tool must connect to</li>
              <li>CI/CD integration: re-scan on every merge to main</li>
              <li>Cloud storage sync: upload DB artifacts to GCS/S3</li>
              <li>Cross-repo semantic search for any AI agent</li>
              <li>New joiners productive on day one &mdash; their AI already knows everything</li>
            </ul>
          </div>
        </div>
      </div>
    </section>
  )
}
