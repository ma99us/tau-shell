# Repository Governance Suite

Documentation and architecture governance prompts for long-lived, AI-assisted
software projects, tailored to TauShell.

## Philosophy

The repository exists to preserve knowledge, not documents. Documents are
expendable; knowledge is not. Reduce accidental complexity, not useful
boundaries. Prefer small, evidence-backed changes that preserve intended behaviour.
The goal is a small, current, trustworthy, AI-friendly knowledge base and a
maintainable codebase, not an arbitrary reduction in files or lines.

## Start here

Read the [usage guide](USAGE-GUIDE.md) for shared safety rules, evidence and trust
definitions, confidence policy, findings contracts, and invocation examples.
Each workflow prompt depends on that guide; supply both to the agent.

### Code governance

1. [Architecture auditor](code-01-architecture-auditor.md): read-only technical
   due diligence and a prioritised remediation plan.
2. Review findings and authorise a bounded set of changes.
3. [Refactoring executor](code-02-refactoring-executor.md): revalidate and execute
   authorised findings, then verify behaviour.
4. [Documentation curator](code-03-documentation-curator.md): synchronise affected
   architecture, decisions, runbooks, and agent context with verified changes.

### Documentation governance

1. Consult the [structure recipe](documentation-00-repository-structure-recipe.md)
   as an optional reference, not a mandatory migration.
2. [Documentation auditor](documentation-01-auditor.md): read-only inventory,
   claim verification, knowledge-gap analysis, and recommendations.
3. Review findings and authorise a bounded set of changes.
4. [Documentation reconciler](documentation-02-reconciler.md): preserve unique
   knowledge while applying authorised updates, merges, and retirements.

The curator follows implementation changes; the reconciler follows a documentation
audit. They share preservation rules but do not perform the same job.

## Core concepts

- **Evidence before assertion:** cite implementation, tests, decisions, and
  approved requirements; distinguish observed behaviour from intended behaviour.
- **Trust is earned:** document type, folder, modification time, and confident
  language do not establish truth.
- **Truth promotion:** move verified, durable knowledge into the appropriate
  maintained artifact without inventing decisions or erasing history.
- **Knowledge preservation:** before removing anything, ask what unique
  knowledge disappears; merge or convert it first.
- **Freshness scoring:** a transparent 0-100 rubric of fitness for purpose,
  not file age (CURRENT, HEALTHY, AGING, AT RISK, ROTTING).
- **Measurable documentation debt:** record drift, redundancy, obsolescence,
  freshness debt, and missing knowledge without gaming scores by deleting files.
- **AI context hygiene:** a concise navigation layer should point to authoritative
  sources, not become another competing source of truth.
- **Compatibility:** content matters more than compliance with a preferred layout.

## Format and portability

These are tool-neutral Markdown workflow prompts, not automatically discoverable
skills, registered slash commands, or enforced tool restrictions. They can be used
with Copilot, Claude Code, Cursor, Windsurf, or another assistant that can read the
repository and follow its instructions.

Paste or attach the relevant prompt and usage guide, or explicitly ask the agent to
read both. For native integration, wrap the workflow in the target tool's supported
prompt or skill format and include the shared guide. A renamed Markdown file alone
does not install a skill. No additional agent or framework is required.

## Canonical sources

These eight Markdown files are the canonical TauShell copy of the suite. Their
examples use this repository's layout (current `docs/v2`, historical `docs/v1`,
future-only `docs/v3`). The generic, shareable version is the
`repository-governance` skill in the ai-cookbook repository; keep the two aligned
when a rule changes. Earlier governance prompts and ZIP archives were retired after
consolidation.
