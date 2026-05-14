export const sections = [
  {
    title: 'Getting Started',
    pages: [
      { slug: '', title: 'Overview', description: 'What Cerebra is and how it works' },
      { slug: 'installation', title: 'Installation', description: 'Install Cerebra in under a minute' },
      { slug: 'quickstart', title: 'Quick Start', description: 'Scan, watch, and serve in 3 commands' },
    ],
  },
  {
    title: 'Using Cerebra',
    pages: [
      { slug: 'scanning', title: 'Scanning', description: 'Index your codebase with embeddings' },
      { slug: 'searching', title: 'Searching', description: 'Semantic and full-text search' },
      { slug: 'mcp-server', title: 'MCP Server', description: 'Connect AI tools via Model Context Protocol' },
      { slug: 'web-ui', title: 'Web UI', description: 'Wiki browser, brain dashboard, and RAG chat' },
    ],
  },
  {
    title: 'Agent Memory',
    pages: [
      { slug: 'brains', title: 'Brains', description: 'Agent conversation indexing and memory' },
      { slug: 'agent-meeting', title: 'Agent Meeting Mode', description: 'Multi-agent structured discussions' },
    ],
  },
  {
    title: 'Reference',
    pages: [
      { slug: 'cli-reference', title: 'CLI Reference', description: 'All commands and flags' },
      { slug: 'configuration', title: 'Configuration', description: 'cerebra.yaml reference' },
    ],
  },
  {
    title: 'Platform Teams',
    pages: [
      { slug: 'ci-cd', title: 'CI/CD Integration', description: 'Auto-reindex on every commit' },
      { slug: 'cloud-storage', title: 'Cloud Storage', description: 'Share the database across your org' },
      { slug: 'rollout-guide', title: 'Rollout Guide', description: 'Ship Cerebra to your whole engineering org' },
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
