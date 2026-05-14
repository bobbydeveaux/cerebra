import { Link } from 'react-router-dom'
import { findPage, findAdjacentPages } from './content.js'

export default function DocPage({ slug, children }) {
  const page = findPage(slug)
  const { prev, next } = findAdjacentPages(slug)

  return (
    <article className="doc-page">
      {page && (
        <header className="doc-header">
          <h1>{page.title}</h1>
          {page.description && <p className="doc-description">{page.description}</p>}
        </header>
      )}
      <div className="doc-content">
        {children}
      </div>
      <nav className="doc-nav-footer">
        {prev ? (
          <Link to={`/docs${prev.slug ? `/${prev.slug}` : ''}`} className="doc-nav-card prev">
            <span className="doc-nav-label">Previous</span>
            <span className="doc-nav-title">{prev.title}</span>
          </Link>
        ) : <div />}
        {next ? (
          <Link to={`/docs${next.slug ? `/${next.slug}` : ''}`} className="doc-nav-card next">
            <span className="doc-nav-label">Next</span>
            <span className="doc-nav-title">{next.title}</span>
          </Link>
        ) : <div />}
      </nav>
    </article>
  )
}

export function Code({ children }) {
  return <pre className="doc-code"><code>{children}</code></pre>
}

export function Table({ headers, rows }) {
  return (
    <div className="doc-table-wrapper">
      <table className="doc-table">
        <thead>
          <tr>{headers.map((h, i) => <th key={i}>{h}</th>)}</tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr key={i}>{row.map((cell, j) => <td key={j}>{cell}</td>)}</tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export function Callout({ type = 'info', children }) {
  return <div className={`doc-callout doc-callout-${type}`}>{children}</div>
}
