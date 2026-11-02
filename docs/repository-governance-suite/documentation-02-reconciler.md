# Documentation Reconciler

## Purpose and boundary

Apply authorised findings from the
[documentation auditor](documentation-01-auditor.md).
The auditor discovers; the reconciler cleans while preserving knowledge.

Read and apply the [usage guide](USAGE-GUIDE.md), especially approval, handoff,
freshness/debt reporting, truth promotion, and context hygiene.
Inputs are the identified audit report, selected `DOC-*` IDs, approval limits,
and the current documentation tree. A recommendation is not permission.

This workflow addresses audit-driven documentation cleanup. Use the
[curator](code-03-documentation-curator.md) for knowledge updates specifically
following verified implementation changes. Do not change implementation to make
documentation match.

## Preflight and autonomy

1. Revalidate each finding, source/destination, current authority, inbound links,
   and relevant implementation evidence. Check for superseding decisions or edits.
2. Preserve unrelated work and repository exclusions. Mark stale findings
   SUPERSEDED, unsafe ones BLOCKED, and unchanged items SKIPPED with reasons.
3. Establish or recover the audit's baseline inventory, freshness components,
   unknown coverage, and debt counting rules.
4. Confirm prerequisites and authorisation before mutating content.

Proceed without repeated approval only for explicitly authorised, high-confidence
(90-100), low-risk actions whose knowledge preservation is established. Ask before
unapproved archive/deletion, uncertain authority changes, wider restructuring, or
newly discovered scope. Lower-confidence or higher-risk actions require the
specific approval described in the guide.

## Knowledge preservation gate

Before replacing, merging, archiving, or deleting material, answer:
**What unique knowledge disappears from the maintained record?**

Protect decision rationale, tradeoff analysis, rejected alternatives, lessons
learned, architectural and operational constraints, and institutional knowledge.

- Identify each unique claim and its source.
- Verify it where possible; preserve uncertainty where it cannot be verified.
- Map it to a maintained destination, an appropriately labelled historical
  reference, or an explicit approved discard rationale.
- MERGE or CONVERT valuable knowledge before retiring its source.
- Retain enough provenance to trace decisions and understand supersession.

Version control alone is not adequate discoverability for knowledge still needed
by maintainers. Conversely, preserving knowledge does not require keeping every
redundant live copy.

## Actions

| Action | Execution requirements |
|---|---|
| UPDATE | Correct evidenced claims, lifecycle labels, missing constraints, and links without silently changing approved requirements. |
| MERGE | Choose one canonical destination, retain all unique valuable knowledge, resolve or label contradictions, then repair references. |
| CONVERT | Promote verified rationale to an appropriately approved ADR, procedures to runbooks, constraints to architecture, or lessons to knowledge references. |
| ARCHIVE | Preserve useful historical context, label status/supersession, and remove misleading current-authority links while keeping selective historical references. |
| DELETE | Require explicit coverage by approval, evidence that no unique needed knowledge remains, and repaired references/tooling consumers. |

KEEP findings need no mutation. ESCALATE findings remain unresolved until the
relevant human decision or evidence is supplied; do not resolve them by guessing.

## Structure and AI context hygiene

Use the [structure recipe](documentation-00-repository-structure-recipe.md) as a
compatibility-aware reference. Create only approved, needed structure. Do not
overwrite distinct content, force folder compliance, or leave contradictory copies.
When moves are authorised, maintain old-to-new and preservation mappings.

Maintain the existing agent navigation/context mechanism. Include links to current
architecture, applicable accepted ADRs, approved active specs, critical invariants,
and validation guidance. Exclude archives, old handoffs, session notes, and completed
plans from default context, not necessarily from the repository. Never include
private notes or secrets. Follow the guide's bounded context template if authorised
to create a new index.

## Validation

Verify final claims, statuses, authority links, preservation mappings, and relative
references. Check navigation and any tooling paths affected by moves. Run existing
documentation checks where available; report unavailable checks and manual coverage
honestly. Ensure user instructions still identify the correct active sources.

Re-score changed units with the same freshness rubric. Report average freshness
before/after and debt before/after using the guide's baseline-inventory rules.
Track merged/converted/archived sources to destinations; report new units,
retirements, UNKNOWNs, and unresolved findings separately. Do not inflate results
by dropping weak units or assigning unverified perfect scores.

## Output

- Per-finding APPLIED/SKIPPED/BLOCKED/SUPERSEDED status and evidence.
- Changed files and action types; preservation and old-to-new mappings.
- Comparable freshness/debt results, denominators, and measurement limitations.
- Checks actually performed, outcomes, residual conflicts, and approval needs.

The goal is small, current, trustworthy, AI-friendly documentation with maximum
useful truth density. Knowledge preservation outweighs superficial tidiness.
