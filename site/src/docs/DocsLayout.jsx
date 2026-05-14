import { useState } from 'react'
import { Routes, Route, NavLink, Link, useLocation } from 'react-router-dom'
import { sections } from './content.js'
import './docs.css'

import Overview from './pages/Overview.jsx'
import Installation from './pages/Installation.jsx'
import QuickStart from './pages/QuickStart.jsx'
import Scanning from './pages/Scanning.jsx'
import Searching from './pages/Searching.jsx'
import McpServer from './pages/McpServer.jsx'
import WebUi from './pages/WebUi.jsx'
import Brains from './pages/Brains.jsx'
import AgentMeetingDoc from './pages/AgentMeeting.jsx'
import CliReference from './pages/CliReference.jsx'
import Configuration from './pages/Configuration.jsx'
import CiCd from './pages/CiCd.jsx'
import CloudStorage from './pages/CloudStorage.jsx'
import RolloutGuide from './pages/RolloutGuide.jsx'

const pageComponents = {
  '': Overview,
  'installation': Installation,
  'quickstart': QuickStart,
  'scanning': Scanning,
  'searching': Searching,
  'mcp-server': McpServer,
  'web-ui': WebUi,
  'brains': Brains,
  'agent-meeting': AgentMeetingDoc,
  'cli-reference': CliReference,
  'configuration': Configuration,
  'ci-cd': CiCd,
  'cloud-storage': CloudStorage,
  'rollout-guide': RolloutGuide,
}

function Sidebar({ open, onClose }) {
  return (
    <>
      {open && <div className="docs-overlay" onClick={onClose} />}
      <aside className={`docs-sidebar ${open ? 'open' : ''}`}>
        <div className="docs-sidebar-header">
          <Link to="/" className="docs-back">
            <span style={{
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: 20,
              height: 20,
              borderRadius: 5,
              background: 'linear-gradient(135deg, #a855f7, #f97316)',
              marginRight: 6,
              verticalAlign: 'middle',
              flexShrink: 0,
            }}>
              <svg width="12" height="12" viewBox="0 0 16 16" fill="none">
                <circle cx="8" cy="5" r="2" fill="#fff"/>
                <circle cx="4" cy="11" r="2" fill="#fff" opacity="0.7"/>
                <circle cx="12" cy="11" r="2" fill="#fff" opacity="0.7"/>
                <line x1="8" y1="5" x2="4" y2="11" stroke="#fff" strokeWidth="1" opacity="0.5"/>
                <line x1="8" y1="5" x2="12" y2="11" stroke="#fff" strokeWidth="1" opacity="0.5"/>
              </svg>
            </span>
            cerebra.stackramp.io
          </Link>
        </div>
        <nav className="docs-nav">
          {sections.map(section => (
            <div key={section.title} className="docs-nav-section">
              <div className="docs-nav-section-title">{section.title}</div>
              {section.pages.map(page => (
                <NavLink
                  key={page.slug}
                  to={`/docs${page.slug ? `/${page.slug}` : ''}`}
                  end={page.slug === ''}
                  className={({ isActive }) => `docs-nav-link ${isActive ? 'active' : ''}`}
                  onClick={onClose}
                >
                  {page.title}
                </NavLink>
              ))}
            </div>
          ))}
        </nav>
        <div className="docs-sidebar-footer">
          <a href="https://github.com/bobbydeveaux/cerebra" target="_blank" rel="noreferrer" className="docs-nav-link external">
            GitHub
          </a>
        </div>
      </aside>
    </>
  )
}

export default function DocsLayout() {
  const [sidebarOpen, setSidebarOpen] = useState(false)

  return (
    <div className="docs-layout">
      <div className="docs-topbar">
        <button className="docs-menu-btn" onClick={() => setSidebarOpen(true)}>
          Docs
        </button>
        <Link to="/" className="docs-topbar-logo">Cerebra</Link>
      </div>
      <Sidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} />
      <main className="docs-main">
        <Routes>
          {Object.entries(pageComponents).map(([slug, Component]) => (
            <Route
              key={slug}
              path={slug === '' ? '/' : slug}
              element={<Component />}
            />
          ))}
        </Routes>
      </main>
    </div>
  )
}
