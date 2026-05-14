import DocPage, { Code, Callout } from '../DocPage.jsx'

export default function AgentMeeting() {
  return (
    <DocPage slug="agent-meeting">
      <p>
        Agent Meeting Mode enables structured multi-agent discussions that produce
        wiki-ready artefacts. Instead of running agents in isolation, you bring multiple
        brains together to collaborate on a specific objective &mdash; each contributing
        context from their indexed sessions.
      </p>

      <Callout type="info">
        <strong>Roadmap feature.</strong> Agent Meeting Mode is planned but not yet implemented.
        The design below describes the intended behaviour and output format. Follow the
        project repository for updates.
      </Callout>

      <h2>How it works</h2>
      <p>
        The user defines a meeting with a title, objective, agenda, required outcomes,
        and a list of invited agents or brains. Cerebra then orchestrates the discussion:
      </p>
      <ol>
        <li><strong>Load brain summaries</strong> &mdash; Cerebra retrieves the summaries and key context for each invited brain</li>
        <li><strong>Agents contribute</strong> &mdash; each agent contributes relevant knowledge from its indexed sessions</li>
        <li><strong>Discussion rounds</strong> &mdash; agents engage in structured rounds, building on each other's contributions</li>
        <li><strong>Generate wiki</strong> &mdash; Cerebra compiles the discussion into a structured wiki-ready document</li>
      </ol>

      <h2>Meeting inputs</h2>
      <p>
        To start a meeting, provide:
      </p>
      <ul>
        <li><strong>Title</strong> &mdash; a clear name for the meeting</li>
        <li><strong>Objective</strong> &mdash; what the meeting should achieve</li>
        <li><strong>Agenda</strong> &mdash; ordered list of topics to cover</li>
        <li><strong>Required outcomes</strong> &mdash; what must be decided or produced</li>
        <li><strong>Invited agents/brains</strong> &mdash; which brains to include in the discussion</li>
      </ul>

      <h2>Meeting outputs</h2>
      <p>
        Every meeting produces a structured document containing:
      </p>
      <ul>
        <li><strong>Title and date</strong> &mdash; meeting identification</li>
        <li><strong>Attendees</strong> &mdash; which brains participated</li>
        <li><strong>Objective</strong> &mdash; restated meeting goal</li>
        <li><strong>Agenda</strong> &mdash; topics covered</li>
        <li><strong>Discussion summary</strong> &mdash; high-level overview of what was discussed</li>
        <li><strong>Agent contributions</strong> &mdash; what each brain brought to the table</li>
        <li><strong>Decisions</strong> &mdash; concrete decisions made during the meeting</li>
        <li><strong>Risks and trade-offs</strong> &mdash; identified concerns and their mitigations</li>
        <li><strong>Action items</strong> &mdash; next steps with owners</li>
        <li><strong>Open questions</strong> &mdash; unresolved items requiring follow-up</li>
      </ul>

      <h2>Use cases</h2>
      <ul>
        <li><strong>Architecture decisions</strong> &mdash; bring together brains that have worked on different parts of the system to evaluate a proposed change</li>
        <li><strong>Incident reviews</strong> &mdash; combine context from the agents that helped debug and fix an incident</li>
        <li><strong>Security audits</strong> &mdash; gather brains with knowledge of auth, networking, and data handling to assess a feature</li>
        <li><strong>Sprint planning</strong> &mdash; use agent context about current state, blockers, and dependencies to plan the next iteration</li>
        <li><strong>Technical due diligence</strong> &mdash; assemble brains familiar with different codebases for acquisition or integration evaluation</li>
      </ul>

      <h2>Output format</h2>
      <p>
        Meeting outputs are wiki-ready Markdown, designed to be committed directly to
        your project's documentation or published to your team wiki. The structured format
        makes meetings searchable and referenceable by future agent sessions.
      </p>
    </DocPage>
  )
}
