export default function HowItWorks() {
  return (
    <section className="section" id="how-it-works">
      <div className="section-label">How It Works</div>
      <h2 className="section-title">Three commands.<br/>That's it.</h2>

      <div className="steps">
        <div className="step">
          <div className="step-number">1</div>
          <div className="step-content">
            <h3>Install Fortress</h3>
            <p>Single Go binary. No Docker. No Node. No Python. Just build and go.</p>
            <div className="terminal">
              <div className="terminal-header">
                <span className="terminal-dot red" />
                <span className="terminal-dot yellow" />
                <span className="terminal-dot green" />
                <span className="terminal-title">Terminal</span>
              </div>
              <div className="terminal-body">
                <span className="prompt">$</span> <span className="cmd">go install github.com/bobbydeveaux/cerebra@latest</span>
              </div>
            </div>
          </div>
        </div>

        <div className="step">
          <div className="step-number">2</div>
          <div className="step-content">
            <h3>Scan Your Codebase</h3>
            <p>Point it at any directory. It recursively scans all repos, chunks by function boundaries, and generates embeddings via Ollama running locally on your machine.</p>
            <div className="terminal">
              <div className="terminal-header">
                <span className="terminal-dot red" />
                <span className="terminal-dot yellow" />
                <span className="terminal-dot green" />
                <span className="terminal-title">Terminal</span>
              </div>
              <div className="terminal-body">
                <span className="prompt">$</span> <span className="cmd">fortress scan ~/code/my-org</span>{'\n'}
                <span className="output">Scanning ....</span>{'\n'}
                <span className="output">Processing <span className="number">1717</span> files...</span>{'\n'}
                <span className="output">Generated <span className="number">4418</span> chunks, embedding...</span>{'\n'}
                <span className="output">Embedding <span className="number">100%</span> <span className="accent">|========================================|</span> (<span className="number">4418</span>/<span className="number">4418</span>) [<span className="number">1m52s</span>]</span>{'\n'}
                {'\n'}
                <span className="success">Done! Jor-El now knows:</span>{'\n'}
                <span className="output">  Repos:      <span className="number">12</span></span>{'\n'}
                <span className="output">  Files:      <span className="number">1,717</span></span>{'\n'}
                <span className="output">  Chunks:     <span className="number">4,418</span></span>{'\n'}
                <span className="output">  Categories: <span className="number">5</span></span>{'\n'}
                <span className="output">  DB size:    <span className="number">95.07 MB</span></span>
              </div>
            </div>
          </div>
        </div>

        <div className="step">
          <div className="step-number">3</div>
          <div className="step-content">
            <h3>Connect Your AI Tools</h3>
            <p>Start the MCP server and add it to Claude Code, Cursor, or any MCP-compatible tool. Your AI now has semantic search across your entire codebase.</p>
            <div className="terminal">
              <div className="terminal-header">
                <span className="terminal-dot red" />
                <span className="terminal-dot yellow" />
                <span className="terminal-dot green" />
                <span className="terminal-title">Terminal</span>
              </div>
              <div className="terminal-body">
                <span className="prompt">$</span> <span className="cmd">fortress serve</span> <span className="comment"># MCP server for AI tools</span>{'\n'}
                <span className="prompt">$</span> <span className="cmd">fortress serve <span className="flag">--ui</span></span> <span className="comment"># Web UI with wiki + chat</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
