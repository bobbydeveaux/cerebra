import { useRef, useEffect } from 'react'

function NodeGraph({ width = 600, height = 500 }) {
  const canvasRef = useRef(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    const dpr = window.devicePixelRatio || 1
    canvas.width = width * dpr
    canvas.height = height * dpr
    ctx.scale(dpr, dpr)

    const nodes = Array.from({ length: 25 }, () => ({
      x: Math.random() * width,
      y: Math.random() * height,
      vx: (Math.random() - 0.5) * 0.4,
      vy: (Math.random() - 0.5) * 0.4,
      r: 3 + Math.random() * 3,
      isCore: Math.random() < 0.15,
    }))

    let animId
    function draw() {
      ctx.clearRect(0, 0, width, height)

      // Update positions
      for (const n of nodes) {
        n.x += n.vx
        n.y += n.vy
        if (n.x < 0 || n.x > width) n.vx *= -1
        if (n.y < 0 || n.y > height) n.vy *= -1
      }

      // Draw connections
      for (let i = 0; i < nodes.length; i++) {
        for (let j = i + 1; j < nodes.length; j++) {
          const dx = nodes[i].x - nodes[j].x
          const dy = nodes[i].y - nodes[j].y
          const dist = Math.sqrt(dx * dx + dy * dy)
          if (dist < 140) {
            const alpha = (1 - dist / 140) * 0.3
            ctx.beginPath()
            ctx.moveTo(nodes[i].x, nodes[i].y)
            ctx.lineTo(nodes[j].x, nodes[j].y)
            ctx.strokeStyle = `rgba(79, 195, 247, ${alpha})`
            ctx.lineWidth = 1
            ctx.stroke()
          }
        }
      }

      // Draw nodes
      for (const n of nodes) {
        ctx.beginPath()
        ctx.arc(n.x, n.y, n.isCore ? n.r * 2 : n.r, 0, Math.PI * 2)
        ctx.fillStyle = n.isCore
          ? 'rgba(79, 195, 247, 0.8)'
          : 'rgba(79, 195, 247, 0.4)'
        ctx.fill()

        if (n.isCore) {
          ctx.beginPath()
          ctx.arc(n.x, n.y, n.r * 3.5, 0, Math.PI * 2)
          ctx.strokeStyle = 'rgba(79, 195, 247, 0.15)'
          ctx.lineWidth = 1
          ctx.stroke()
        }
      }

      animId = requestAnimationFrame(draw)
    }
    draw()
    return () => cancelAnimationFrame(animId)
  }, [width, height])

  return (
    <canvas
      ref={canvasRef}
      className="hero-canvas"
      style={{ width, height }}
    />
  )
}

export default function Hero() {
  return (
    <>
      <nav className="navbar">
        <a href="#" className="nav-logo">
          <svg width="28" height="28" viewBox="0 0 28 28" fill="none">
            <rect x="2" y="6" width="24" height="18" rx="3" stroke="#4fc3f7" strokeWidth="2" fill="none"/>
            <path d="M8 2v4M20 2v4M14 2v4" stroke="#4fc3f7" strokeWidth="2" strokeLinecap="round"/>
            <path d="M8 14h12M8 18h8" stroke="#4fc3f7" strokeWidth="1.5" strokeLinecap="round" opacity="0.6"/>
            <circle cx="21" cy="17" r="4" fill="#050505" stroke="#4fc3f7" strokeWidth="1.5"/>
            <circle cx="21" cy="17" r="1.5" fill="#4fc3f7"/>
          </svg>
          Fortress
        </a>
        <div className="nav-links">
          <a href="#problem">The Problem</a>
          <a href="#solution">Solution</a>
          <a href="#features">Features</a>
          <a href="#platform-teams">Platform Teams</a>
          <a href="/docs">Docs</a>
          <a className="nav-cta" href="/docs/quickstart">Get Started</a>
        </div>
      </nav>

      <section className="hero">
        <NodeGraph />
        <div className="hero-content">
          <div className="hero-badge">
            <span className="hero-badge-dot" />
            Open Source &middot; Run Locally &middot; Own Your Data
          </div>

          <h1>Your AI is <span className="highlight">flying blind.</span><br/>That's on you.</h1>

          <p className="hero-sub">
            Every AI tool your engineers use &mdash; Copilot, Claude, Cursor, Windsurf &mdash; knows nothing about your codebase. Fortress fixes that. One scan. Full embeddings. Instant context for every AI interaction.
          </p>

          <div className="hero-buttons">
            <a href="#get-started" className="btn-primary">Get Started Free</a>
            <a href="#how-it-works" className="btn-secondary">See How It Works</a>
          </div>

          <div className="hero-stats">
            <div className="hero-stat">
              <div className="hero-stat-value">100%</div>
              <div className="hero-stat-label">Local & Private</div>
            </div>
            <div className="hero-stat">
              <div className="hero-stat-value">~2 min</div>
              <div className="hero-stat-label">Per 1,700 files</div>
            </div>
            <div className="hero-stat">
              <div className="hero-stat-value">MCP</div>
              <div className="hero-stat-label">Native Protocol</div>
            </div>
          </div>
        </div>
      </section>
    </>
  )
}
