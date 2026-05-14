import { useRef, useEffect } from 'react'

function NeuralCanvas() {
  const ref = useRef(null)

  useEffect(() => {
    const canvas = ref.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    const W = 400, H = 400
    canvas.width = W
    canvas.height = H

    const nodes = Array.from({ length: 30 }, () => ({
      x: Math.random() * W,
      y: Math.random() * H,
      vx: (Math.random() - 0.5) * 0.3,
      vy: (Math.random() - 0.5) * 0.3,
      r: Math.random() * 2.5 + 1.5,
      hue: Math.random() > 0.5 ? 270 : 25,
    }))

    const pulses = []

    let animId
    function draw() {
      ctx.clearRect(0, 0, W, H)

      nodes.forEach(n => {
        n.x += n.vx
        n.y += n.vy
        if (n.x < 0 || n.x > W) n.vx *= -1
        if (n.y < 0 || n.y > H) n.vy *= -1
      })

      for (let i = 0; i < nodes.length; i++) {
        for (let j = i + 1; j < nodes.length; j++) {
          const dx = nodes[i].x - nodes[j].x
          const dy = nodes[i].y - nodes[j].y
          const dist = Math.sqrt(dx * dx + dy * dy)
          if (dist < 120) {
            const alpha = (1 - dist / 120) * 0.15
            ctx.beginPath()
            ctx.moveTo(nodes[i].x, nodes[i].y)
            ctx.lineTo(nodes[j].x, nodes[j].y)
            ctx.strokeStyle = `rgba(168, 85, 247, ${alpha})`
            ctx.lineWidth = 1
            ctx.stroke()
          }
        }
      }

      if (Math.random() < 0.02) {
        const a = nodes[Math.floor(Math.random() * nodes.length)]
        const b = nodes[Math.floor(Math.random() * nodes.length)]
        if (a !== b) {
          pulses.push({ ax: a.x, ay: a.y, bx: b.x, by: b.y, t: 0 })
        }
      }

      for (let i = pulses.length - 1; i >= 0; i--) {
        const p = pulses[i]
        p.t += 0.025
        if (p.t > 1) { pulses.splice(i, 1); continue }
        const x = p.ax + (p.bx - p.ax) * p.t
        const y = p.ay + (p.by - p.ay) * p.t
        const alpha = Math.sin(p.t * Math.PI) * 0.8
        ctx.beginPath()
        ctx.arc(x, y, 3, 0, Math.PI * 2)
        ctx.fillStyle = `rgba(249, 115, 22, ${alpha})`
        ctx.fill()
      }

      nodes.forEach(n => {
        ctx.beginPath()
        ctx.arc(n.x, n.y, n.r, 0, Math.PI * 2)
        const color = n.hue === 270
          ? `rgba(168, 85, 247, 0.6)`
          : `rgba(249, 115, 22, 0.6)`
        ctx.fillStyle = color
        ctx.fill()
      })

      animId = requestAnimationFrame(draw)
    }

    draw()
    return () => cancelAnimationFrame(animId)
  }, [])

  return <canvas ref={ref} className="hero-canvas" width="400" height="400" />
}

export default function Hero() {
  return (
    <>
      <nav className="navbar">
        <a href="/" className="nav-logo">
          <span className="nav-logo-icon">
            <svg viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
              <circle cx="8" cy="5" r="2" fill="#fff"/>
              <circle cx="4" cy="11" r="2" fill="#fff" opacity="0.7"/>
              <circle cx="12" cy="11" r="2" fill="#fff" opacity="0.7"/>
              <line x1="8" y1="5" x2="4" y2="11" stroke="#fff" strokeWidth="1" opacity="0.5"/>
              <line x1="8" y1="5" x2="12" y2="11" stroke="#fff" strokeWidth="1" opacity="0.5"/>
              <line x1="4" y1="11" x2="12" y2="11" stroke="#fff" strokeWidth="1" opacity="0.5"/>
            </svg>
          </span>
          Cerebra
        </a>
        <div className="nav-links">
          <a href="#problem">The Problem</a>
          <a href="#solution">Solution</a>
          <a href="#features">Features</a>
          <a href="#architecture">Architecture</a>
          <a href="/docs">Docs</a>
          <a href="/docs/quickstart" className="nav-cta">Get Started</a>
        </div>
      </nav>

      <section className="hero">
        <div className="hero-glow" />
        <div className="hero-glow-2" />
        <NeuralCanvas />
        <div className="hero-content">
          <div className="hero-badge">
            <span className="hero-badge-dot" />
            Open Source &middot; Local First &middot; Agent Agnostic
          </div>

          <h1>
            Your agents<br />
            <span className="highlight">forget everything.</span>
          </h1>

          <p className="hero-sub">
            Every AI session starts from zero. Cerebra changes that.
            Persistent memory for your entire agent fleet &mdash; every conversation
            indexed, summarised, and shared in real time.
          </p>

          <div className="hero-buttons">
            <a href="/docs/quickstart" className="btn-primary">
              Get Started
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M3 8h10M9 4l4 4-4 4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/></svg>
            </a>
            <a href="https://github.com/bobbydeveaux/cerebra" className="btn-secondary">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>
              View on GitHub
            </a>
          </div>

          <div className="hero-stats">
            <div className="hero-stat">
              <div className="hero-stat-value violet">Real-time</div>
              <div className="hero-stat-label">Conversation Indexing</div>
            </div>
            <div className="hero-stat">
              <div className="hero-stat-value coral">Cross-Agent</div>
              <div className="hero-stat-label">Context Discovery</div>
            </div>
            <div className="hero-stat">
              <div className="hero-stat-value gold">MCP Native</div>
              <div className="hero-stat-label">Protocol Support</div>
            </div>
          </div>
        </div>
      </section>
    </>
  )
}
