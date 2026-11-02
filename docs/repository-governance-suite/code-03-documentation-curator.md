# Code Documentation Curator

## Purpose and scope

Keep architecture and engineering knowledge aligned with verified implementation
changes. This workflow follows the
[refactoring executor](code-02-refactoring-executor.md); it is not a general
documentation purge or repository migration.

Read and apply the [usage guide](USAGE-GUIDE.md): authority, evidence, approval,
truth promotion, AI context, and preservation rules. For whole-documentation
cleanup, use the [auditor](documentation-01-auditor.md) and
[reconciler](documentation-02-reconciler.md) instead.

## Inputs

- Relevant implementation diff and executor outcome, including checks and limits.
- Architecture findings, approved requirements, accepted ADRs, and existing docs.
- Repository instructions and the current documentation authority/navigation map.

Revalidate affected claims. If the implementation is untested or a finding remains
blocked, retain that limitation. Do not turn a proposed or partially applied change
into a documented success.

## Truth promotion

Important durable knowledge should not remain trapped in permitted low-trust
sources such as plans, meeting notes, or AI handoffs. Do not inspect private or
excluded scratch files to find it.

| Knowledge | Appropriate maintained destination |
|---|---|
| Verified architecture boundaries and implementation constraints | Current architecture documentation. |
| Decision rationale, tradeoffs, and rejected alternatives | An ADR with accurate status and decision-owner approval. |
| Verified operational procedures and recovery steps | A runbook or recipe. |
| Supported lessons and research conclusions | A knowledge reference with evidence and applicability limits. |
| Navigation, active authority, critical invariants, validation links | The existing agent context/index mechanism. |

A plan does not automatically become an accepted ADR. Do not invent rationale,
retroactively assert approval, or rewrite an accepted historical decision to hide
its replacement. Propose a new/superseding decision when appropriate.

## Responsibilities

1. Map each verified change to affected architecture, ADRs, runbooks, active specs,
   developer/user documentation, and context links.
2. Update only the affected knowledge and directly related navigation. Describe
   behaviour accurately, including constraints, error paths, and unproven areas.
3. Keep requirements distinct from implementation status. Escalate disagreements
   rather than silently changing approved scope to match the code.
4. Preserve rationale, tradeoffs, lessons, rejected alternatives, and institutional
   constraints before replacing text.
5. Update existing AI context documents rather than create a competing summary.
   Follow the guide's one-page, link-first template where a new index is authorised.
6. Use the [structure recipe](documentation-00-repository-structure-recipe.md) only
   when useful. Content is the priority; do not force migration or create empty
   structures, ADRs, or context files to satisfy the recipe.

## Validation and output

Verify edited claims against actual evidence, check relative links and navigation,
and run existing documentation checks where available and applicable. Do not claim
tests passed solely because an executor expected them to pass.

Return:

- Changed documents mapped to verified implementation changes.
- Promoted knowledge with source, destination, evidence, and decision status.
- Link/documentation checks and their actual results.
- Unresolved contradictions, ownership/approval questions, and known limitations.

The outcome is a trustworthy architectural knowledge base, not a larger collection
of summaries. Archive or delete unrelated material only through a separately
authorised reconciliation.
