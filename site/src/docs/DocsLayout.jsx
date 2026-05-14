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
            <svg width="20" height="20" viewBox="0 0 28 28" fill="none" style={{verticalAlign: 'middle', marginRight: '6px'}}>
              <rect x="2" y="6" width="24" height="18" rx="3" stroke="#4fc3f7" strokeWidth="2" fill="none"/>
              <path d="M8 2v4M20 2v4M14 2v4" stroke="#4fc3f7" strokeWidth="2" strokeLinecap="round"/>
              <circle cx="21" cy="17" r="4" fill="#0a0a0a" stroke="#4fc3f7" strokeWidth="1.5"/>
              <circle cx="21" cy="17" r="1.5" fill="#4fc3f7"/>
            </svg>
            fortress.stackramp.io
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
        <Link to="/" className="docs-topbar-logo">Fortress</Link>
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
