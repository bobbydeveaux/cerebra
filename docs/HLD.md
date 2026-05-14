# Cerebra — High-Level Design (HLD)

**Version:** 1.0
**Status:** Draft

---

## 1. Overview

Cerebra is a persistent AI agent memory and orchestration layer built on top of [Fortress](~/code/newlife/fortress). While Fortress indexes codebases and Confluence spaces into a vector-backed knowledge base, Cerebra extends this by treating AI agent conversations as first-class indexable knowledge sources.

Each agent session becomes a **"brain"** — a combination of conversation history, summaries, decisions, code changes, and task context. Cerebra watches these conversations in real time, indexes them incrementally, generates concise summaries, and exposes the combined knowledge through Fortress's MCP endpoint and a dedicated UI.

This enables multiple agents to retain long-term context, discover each other's work, and coordinate through a shared knowledge layer — without constantly passing huge prompts between sessions.

**Name origin:** Cerebra is the plural of cerebrum — multiple brains, unified.

---

## 2. Goals

1. **Persist useful context** across Claude Code (or other AI coding) sessions
2. **Index conversation/session files** incrementally as they change
3. **Generate concise summaries** per conversation to reduce token usage
4. **Expose searchable agent memory** via Fortress MCP
5. **Build a UI** showing active and historical "agent brains"
6. **Enable cross-agent awareness** — agents discover relevant context from other agents
7. **Support agent meetings** — structured multi-agent discussions producing wiki artefacts
8. **Avoid model lock-in** by supporting multiple agent tools over time

---

## 3. Relationship to Fortress

Cerebra is **not a replacement** for Fortress — it is a layer that extends Fortress's capabilities.

```
┌──────────────────────────────────────────────────────────┐
│                        CEREBRA                            │
│                                                          │
│  Agent Memory · Brain Registry · Meeting Mode · UI       │
│                                                          │
├──────────────────────────────────────────────────────────┤
│                       FORTRESS                            │
│                                                          │
│  Scanner · Chunker · Embedder · Vector Store · MCP · UI  │
│  (codebases, Confluence, git history)                    │
└──────────────────────────────────────────────────────────┘
```

Fortress provides:
- Incremental indexing engine
- Vector store (Jor-El / SQLite-vec)
- Embedding pipeline (Ollama / OpenAI)
- MCP server
- Query engine (vector similarity + FTS5)

Cerebra adds:
- Filesystem watcher for agent conversation directories
- Conversation parsing and ingestion
- Summary generation layer
- Agent Brain Registry
- Extended MCP tools for agent memory
- Brain-aware UI
- Cross-agent context discovery
- Agent Meeting Mode

---

## 4. Architecture Diagram

```
                          ┌─────────────────────┐
                          │   Agent Sessions     │
                          │                      │
                          │  Claude Code         │
                          │  Other AI Agents     │
                          └──────────┬──────────┘
                                     │
                          conversation files
                    (~/.claude/projects/*/*.jsonl)
                                     │
                                     ▼
                          ┌─────────────────────┐
                          │  Filesystem Watcher  │
                          │    (fsnotify/Go)     │
                          └──────────┬──────────┘
                                     │
                            detect create/modify
                                     │
                                     ▼
                          ┌─────────────────────┐
                          │ Conversation Ingest  │
                          │                      │
                          │  Parse → Normalise   │
                          │  → Extract metadata  │
                          └──────────┬──────────┘
                                     │
                    ┌────────────────┴────────────────┐
                    │                                  │
                    ▼                                  ▼
         ┌──────────────────┐              ┌──────────────────┐
         │ Fortress Indexer  │              │  Summary Layer   │
         │                  │              │                  │
         │  Chunker →       │              │  Short summary   │
         │  Embedder →      │              │  Decision summary│
         │  Vector Store    │              │  Task summary    │
         └────────┬─────────┘              └────────┬─────────┘
                  │                                  │
                  └────────────┬─────────────────────┘
                               │
                               ▼
                    ┌──────────────────────┐
                    │  Agent Brain Registry │
                    │                       │
                    │  brain_id, agent_type, │
                    │  project, status,      │
                    │  summaries, relations  │
                    └──────────┬────────────┘
                               │
              ┌────────────────┼────────────────┐
              │                │                │
              ▼                ▼                ▼
   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
   │  MCP Server   │  │  Cerebra UI  │  │   Meeting    │
   │  (extended)   │  │              │  │   Engine     │
   │               │  │  Dashboards  │  │              │
   │ search_brain  │  │  Brain list  │  │  Multi-agent │
   │ list_brains   │  │  Summaries   │  │  discussions │
   │ get_summary   │  │  Cross-refs  │  │  Wiki output │
   │ get_related   │  │  Search      │  │              │
   └──────────────┘  └──────────────┘  └──────────────┘
```

---

## 5. Core Components

### 5.1 Filesystem Watcher

Watches Claude Code's conversation storage for file changes using **`fsnotify`** (Go library). This is the same library Fortress already uses for its `fortress watch` mode — it uses FSEvents on macOS, inotify on Linux, and ReadDirectoryChangesW on Windows, so Cerebra is cross-platform out of the box.

**Claude Code file layout (discovered):**

```
~/.claude/
├── projects/                              ← PRIMARY WATCH TARGET
│   ├── -Users-bobby-code-project-x/       ← one dir per project (path with / → -)
│   │   ├── <session-id>.jsonl             ← conversation log (JSONL, one msg per line)
│   │   ├── <session-id>/
│   │   │   ├── subagents/                 ← sub-agent conversations
│   │   │   │   ├── agent-<id>.jsonl
│   │   │   │   └── agent-<id>.meta.json
│   │   │   └── tool-results/              ← large tool outputs stored separately
│   │   │       └── <hash>.txt
│   │   └── memory/                        ← Claude Code's own memory files
│   └── -Users-bobby-code-project-y/
│       └── ...
├── sessions/                              ← session index
│   └── <pid>.json                         ← maps PID → session ID, CWD, start time
└── history.jsonl                          ← global prompt history (what user typed)
```

**Conversation JSONL line types:**

| `type` field | Content |
|-------------|---------|
| `"permission-mode"` | Session configuration |
| `"file-history-snapshot"` | File state snapshots for undo |
| `"user"` | User messages (includes CWD, git branch, timestamp, session ID) |
| (no type) | Assistant responses, tool calls, tool results |

**Watched paths:**
- `~/.claude/projects/*/` — detect new/modified `*.jsonl` conversation files
- `~/.claude/projects/*/*/subagents/` — sub-agent conversation logs
- `~/.claude/sessions/` — session index for mapping PIDs to projects

**Incremental strategy:**
- Track last byte offset per JSONL file — on change, read only the new lines appended since last index
- JSONL is append-only during a session, making tail-reading reliable
- Use `content_hash` on completed sessions to avoid reprocessing

**Responsibilities:**
- Detect new conversation/session files
- Detect updates (new lines appended) to active sessions
- Trigger incremental indexing via Fortress
- Avoid reprocessing unchanged content

### 5.2 Conversation Ingestion

Parses Claude Code JSONL conversation files into structured records with a common schema.

**Source mapping (Claude Code):**

The project directory name encodes the working directory path (`/` → `-`), e.g.:
- `-Users-bobby-code-newlife-fortress` → `/Users/bobby/code/newlife/fortress`

The session file is named `<session-id>.jsonl`. Each line is a JSON object. The session index at `~/.claude/sessions/<pid>.json` provides:

```json
{
  "pid": 94413,
  "sessionId": "19f4f8d4-5493-4496-8c54-684815e48fe4",
  "cwd": "/Users/bobby/code/newlife/cerebra",
  "startedAt": 1777276371601,
  "kind": "interactive",
  "entrypoint": "cli"
}
```

**Normalised metadata per conversation (common schema):**

```yaml
agent: claude-code
session_id: 19f4f8d4-5493-4496-8c54-684815e48fe4
project_path: /Users/bobby/code/newlife/fortress
project_key: -Users-bobby-code-newlife-fortress
created_at: 2026-04-27T08:00:00Z
updated_at: 2026-04-27T08:30:00Z
source_file: ~/.claude/projects/-Users-bobby-code-newlife-fortress/19f4f8d4-....jsonl
git_branch: main
entrypoint: cli
has_subagents: true
```

The ingestion layer normalises different agent formats (Claude Code first, Cursor/Copilot/etc. later) into this common schema so the rest of the pipeline is agent-agnostic.

**Indexed content per conversation:**
- User prompts (`type: "user"`)
- Assistant responses (tool calls, reasoning, code generation)
- Tool outputs (inline and from `tool-results/` directory)
- Sub-agent conversations (from `subagents/` directory)
- Code decisions and errors encountered
- Links to repos/docs/branches/PRs (extracted from message metadata)

Chunking prioritises semantic usefulness over raw token size.

### 5.3 Summary Layer

Each time a conversation is updated, Cerebra generates or updates a compact summary. This is the key mechanism for keeping agent prompts concise while preserving important context.

**Summary types:**

| Type | Purpose | Example |
|------|---------|---------|
| Short summary | 1-3 paragraph overview | "This agent worked on adding GitHub webhook handling to Fortress..." |
| Decision summary | Key architectural choices made | "Decided to use event-driven ingestion rather than polling" |
| Task summary | What was done / what remains | "Completed: webhook handler. Remaining: payload signature validation" |
| Project memory | Durable reusable context | "Auth headers must be preserved through the edge proxy" |

Summaries are regenerated or patched incrementally — not rebuilt from scratch on every update.

### 5.4 Agent Brain Registry

A registry tracking every known conversation/brain. This is the foundation for the UI and cross-agent discovery.

**Brain record:**

```yaml
brain_id: claude-project-x-20260427
agent_type: claude-code
project: project-x
status: active
last_seen: 2026-04-27T08:45:00Z
summary: Added webhook-based indexing flow.
source_path: ~/.claude/...
related_repos:
  - project-x
related_docs:
  - Confluence/Fortress Architecture
```

### 5.5 MCP Interface (Extended)

Cerebra extends the existing Fortress MCP server with agent memory tools.

**New tools:**

| Tool | Input | Output |
|------|-------|--------|
| `search_brain` | `query: string, project?: string` | Relevant agent context chunks ranked by relevance |
| `list_brains` | `project?: string, status?: string` | Array of known brain records |
| `get_brain_summary` | `brain_id: string` | Full summary of a specific brain |
| `get_related_context` | `project_path: string` | Combined context: repo + docs + agent summaries |
| `get_recent_agent_activity` | `repo: string` | Recent agent actions in a given repo |

**Example agent flow:**

```
Claude Code starts in repo X
       ↓
Calls Fortress MCP (Cerebra-extended)
       ↓
Fortress returns:
  - relevant repo context (existing)
  - Confluence context (existing)
  - previous agent summaries (NEW)
  - related decisions (NEW)
       ↓
Claude continues with much richer continuity
```

### 5.6 Cerebra UI

A web interface for visualising and managing agent brains.

**Views:**

```
Dashboard
 ├── Projects
 ├── Repositories
 ├── Agent Brains
 ├── Confluence Spaces
 ├── Recent Decisions
 └── Search
```

**Per-brain view:**

| Field | Description |
|-------|-------------|
| Brain Name | Human-readable identifier |
| Agent Type | claude-code, cursor, etc. |
| Project | Associated project/repo |
| Last Updated | Timestamp of last activity |
| Summary | Current concise summary |
| Linked Conversations | Related brain sessions |
| Linked Code | Repos, branches, PRs |
| Linked Docs | Confluence pages, wikis |
| Open in original agent | Deep-link back to the agent session |
| Search within brain | Scoped semantic search |

**Tech approach:** Extends Fortress's existing htmx-based web UI — no JavaScript framework, no build step.

---

## 6. Cross-Agent Awareness

Agents do **not** blindly receive all other agent context. Instead, they query Fortress/Cerebra for relevant context on demand.

**Example:**

```
Agent A is working on auth middleware.

Fortress/Cerebra finds:
  - Agent B recently changed JWT validation in auth-service
  - Agent C discussed API gateway routing constraints
  - Confluence has an auth architecture page

Cerebra returns a concise context packet:

  Relevant Agent Context:
  1. Agent B changed JWT validation in repo auth-service.
  2. Agent C identified API gateway routing constraints.
  3. Confluence says auth headers must be preserved through the edge proxy.
```

This gives coordination without huge prompts. Relevance is determined by:
- Project/repo overlap
- Semantic similarity of queries to brain summaries
- Recency weighting
- Explicit agent queries

---

## 7. Token Efficiency Strategy

The system avoids "infinite context dumping". Instead:

1. **Summaries first** — always prefer summaries over raw conversation chunks
2. **Raw chunks only when needed** — on explicit deep-dive queries
3. **Relevance scoring** — vector similarity filters out noise
4. **Recency weighting** — recent context ranks higher
5. **Project/repo filters** — scope queries to relevant projects
6. **Explicit agent queries** — agents ask for what they need, not everything
7. **Durable memory extraction** — promote important facts to long-lived memory

**The goal is not to keep every token alive. The goal is to preserve useful working memory.**

---

## 8. Agent Meeting Mode

Cerebra supports a "meeting mode" where multiple agent brains are invited into a structured discussion around a defined agenda.

### 8.1 Meeting Input

The user provides:
- Meeting title
- Objective
- Agenda items
- Required outcomes
- Relevant repos/docs/context
- Invited agents/brains
- Output format

### 8.2 Meeting Flow

```
User defines meeting
       ↓
Cerebra loads invited brain summaries + relevant context
       ↓
Each agent contributes from its own context
       ↓
Meeting orchestrator manages discussion rounds
       ↓
Agents challenge, refine, and resolve points
       ↓
Cerebra generates final meeting wiki
```

### 8.3 Meeting Output (Wiki)

Each meeting produces a durable, wiki-ready document:

```markdown
# Agent Meeting: [Title]

## Objective
[Stated goal]

## Attendees
- [Agent 1] (role/expertise)
- [Agent 2] (role/expertise)

## Agenda
1. [Item 1]
2. [Item 2]

## Discussion Summary
[Narrative of the discussion]

## Agent-by-Agent Contributions
### [Agent 1]
[Key points and arguments]

### [Agent 2]
[Key points and arguments]

## Decisions
[What was agreed]

## Risks & Trade-offs
[Identified risks and trade-off analysis]

## Action Items
- [ ] [Action 1] — owner: [Agent/Human]
- [ ] [Action 2] — owner: [Agent/Human]

## Open Questions
[Unresolved items for follow-up]
```

Output format: Markdown (default) or Confluence wiki format.

### 8.4 Meeting Use Cases

- Architecture decisions
- Sprint planning
- Incident reviews
- Security reviews
- PR strategy
- Refactoring plans
- Product discovery
- Technical due diligence

This is essentially **AI architecture council mode**.

---

## 9. Event Flow (End-to-End)

```
1. Claude Code (or other agent) creates or updates a session file
2. Cerebra's filesystem watcher detects the change
3. Conversation ingestion layer parses the changed content
4. New chunks are sent to Fortress's indexing pipeline
5. Summary layer generates or patches the conversation summary
6. Brain Registry is updated (status, last_seen, summary)
7. Extended MCP endpoint exposes the latest context
8. Cerebra UI reflects the updated brain state
9. Other agents can now discover this context via MCP queries
```

---

## 10. MVP Definition

The first useful version should be deliberately simple:

1. Watch Claude Code conversation directory (`~/.claude/`)
2. Detect new or changed session files
3. Incrementally index conversation text via Fortress
4. Generate/update one summary per session
5. Expose `search_brain` and `list_brains` via MCP
6. Add a basic UI table of known brains

**MVP success criteria:**
> A new Claude Code session can discover useful context from a previous Claude Code session — without manually copying prompts or summaries.

---

## 11. Future Enhancements

- GitHub branch/PR awareness per brain
- Agent conflict detection (two agents modifying the same files)
- Project-level decision logs
- Automatic "memory promotion" (important facts → durable memory)
- Slack/Teams ingestion
- Jira/Linear ticket linkage
- Multi-model support (not just Claude)
- Agent-to-agent handoff
- Event-driven orchestration
- Token usage/cost tracking per agent
- Timeline view of project thinking
- Cursor / Copilot / Windsurf agent support

---

## 12. Strategic Value

Cerebra transforms Fortress from a code/documentation indexer into a **persistent AI engineering memory system**:

```
  Codebase knowledge        (Fortress — existing)
+ Documentation knowledge   (Fortress — existing)
+ Conversation knowledge    (Cerebra — new)
+ Agent activity knowledge  (Cerebra — new)
+ Agent collaboration       (Cerebra — meeting mode)
────────────────────────────────────────────────────
= Engineering Brain
```

This is the foundation for a **multi-agent software delivery platform**.
