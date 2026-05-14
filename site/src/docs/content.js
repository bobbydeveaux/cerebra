export const sections = [
  {
    title: 'Getting Started',
    pages: [
      { slug: '', title: 'Overview', description: 'What Fortress is and how it works' },
      { slug: 'installation', title: 'Installation', description: 'Install Fortress in under a minute' },
      { slug: 'quickstart', title: 'Quick Start', description: 'Scan, search, and serve in 3 commands' },
    ],
  },
  {
    title: 'Using Fortress',
    pages: [
      { slug: 'scanning', title: 'Scanning', description: 'Index your codebase with embeddings' },
      { slug: 'searching', title: 'Searching', description: 'Semantic and full-text search' },
      { slug: 'mcp-server', title: 'MCP Server', description: 'Connect AI tools via Model Context Protocol' },
      { slug: 'web-ui', title: 'Web UI', description: 'Wiki browser and RAG chat interface' },
    ],
  },
  {
    title: 'Reference',
    pages: [
      { slug: 'cli-reference', title: 'CLI Reference', description: 'All commands and flags' },
      { slug: 'configuration', title: 'Configuration', description: 'fortress.yaml reference' },
    ],
  },
  {
    title: 'Platform Teams',
    pages: [
      { slug: 'ci-cd', title: 'CI/CD Integration', description: 'Auto-reindex on every commit' },
      { slug: 'cloud-storage', title: 'Cloud Storage', description: 'Share the database across your org' },
      { slug: 'rollout-guide', title: 'Rollout Guide', description: 'Ship Fortress to your whole engineering org' },
    ],
  },
]

export const allPages = sections.flatMap(s => s.pages)

export function findPage(slug) {
  return allPages.find(p => p.slug === slug)
}

export function findAdjacentPages(slug) {
  const idx = allPages.findIndex(p => p.slug === slug)
  return {
    prev: idx > 0 ? allPages[idx - 1] : null,
    next: idx < allPages.length - 1 ? allPages[idx + 1] : null,
  }
}
