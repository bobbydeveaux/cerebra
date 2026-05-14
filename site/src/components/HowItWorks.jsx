export default function HowItWorks() {
  return (
    <section className="section" id="how-it-works">
      <div className="section-label">Getting Started</div>
      <h2 className="section-title">
        Three steps to <span className="text-violet">total recall.</span>
      </h2>
      <p className="section-subtitle">
        Single Go binary. No Docker. No Node. No Python. Get persistent agent memory
        running in under a minute.
      </p>

      <div className="steps">
        <div className="step">
          <div className="step-number">1</div>
          <div className="step-content">
            <h3>Install Cerebra</h3>
            <p>One command. One binary. Cross-platform.</p>
            <div className="terminal">
              <div className="terminal-header">
                <span className="terminal-dot red" />
                <span className="terminal-dot yellow" />
                <span className="terminal-dot green" />
                <span className="terminal-title">terminal</span>
              </div>
              <div className="terminal-body">
<span className="prompt">$ </span><span className="cmd">go install github.com/bobbydeveaux/cerebra@latest</span>
              </div>
            </div>
          </div>
        </div>

        <div className="step">
          <div className="step-number">2</div>
          <div className="step-content">
            <h3>Scan &amp; Watch</h3>
            <p>Index your codebase and start watching agent conversations.</p>
            <div className="terminal">
              <div className="terminal-header">
                <span className="terminal-dot red" />
                <span className="terminal-dot yellow" />
                <span className="terminal-dot green" />
                <span className="terminal-title">terminal</span>
              </div>
              <div className="terminal-body">
<span className="comment"># Index your codebase</span>{'\n'}
<span className="prompt">$ </span><span className="cmd">cerebra scan ~/code/my-org</span>{'\n'}
<span className="output">Scanning 12 repositories...</span>{'\n'}
<span className="output">  Files:    </span><span className="number">1,717</span>{'\n'}
<span className="output">  Chunks:   </span><span className="number">4,418</span>{'\n'}
<span className="output">  Duration: </span><span className="number">1m52s</span>{'\n'}
<span className="success">Done.</span>{'\n'}
{'\n'}
<span className="comment"># Start watching agent conversations</span>{'\n'}
<span className="prompt">$ </span><span className="cmd">cerebra brains watch</span>{'\n'}
<span className="output">Watching ~/.claude/projects/ for conversations...</span>{'\n'}
<span className="success">&#x2713; Indexed 3 new sessions</span>
              </div>
            </div>
          </div>
        </div>

        <div className="step">
          <div className="step-number">3</div>
          <div className="step-content">
            <h3>Connect Your Agents</h3>
            <p>Start the MCP server and every AI tool gets instant access to the collective memory.</p>
            <div className="terminal">
              <div className="terminal-header">
                <span className="terminal-dot red" />
                <span className="terminal-dot yellow" />
                <span className="terminal-dot green" />
                <span className="terminal-title">terminal</span>
              </div>
              <div className="terminal-body">
<span className="comment"># MCP server for Claude Code / Cursor</span>{'\n'}
<span className="prompt">$ </span><span className="cmd">cerebra serve</span>{'\n'}
<span className="output">MCP server listening on stdio...</span>{'\n'}
{'\n'}
<span className="comment"># Or with the web UI</span>{'\n'}
<span className="prompt">$ </span><span className="cmd">cerebra serve <span className="flag">--ui</span></span>{'\n'}
<span className="output">Web UI:  </span><span className="accent">http://localhost:8080</span>{'\n'}
<span className="output">Wiki, chat, brain dashboard ready.</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
