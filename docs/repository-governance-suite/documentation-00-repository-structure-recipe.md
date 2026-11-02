# Repository Structure Recipe

## Purpose and operating mode

A reference for an AI-friendly knowledge layout, not a required repository
standard. Read the [usage guide](USAGE-GUIDE.md) first.

Consulting this recipe does not authorise creating folders, moving files, or
changing documentation. Assess the current layout read-only; migrate only under an
explicitly approved plan, normally through the
[documentation reconciler](documentation-02-reconciler.md).

Classify by content, declared lifecycle, and authority. Folder names are hints,
not proof. Knowledge preservation outweighs folder compliance.

## Recommended layout

Use existing equivalent locations. Introduce a directory only when it has a
concrete maintained artifact to hold.

```text
docs/
  architecture/
    adrs/
  planning/
    roadmap/
    active-plans/
    backlog/
    technical-debt/
  specs/
    active/
    proposed/
  recipes/
  knowledge/
    lessons-learned/
    research/
    tradeoff-analysis/
  audit/
    architecture/
    documentation/
    governance/
  archive/
AI_CONTEXT.md
```

The example context file is at the repository root. An existing equivalent index
elsewhere is equally valid; follow repository conventions rather than duplicate it.

## Content roles

| Location | Question answered |
|---|---|
| Architecture | What exists, how is it bounded, and what constraints apply? |
| ADRs | Which decision was made, why, with what status, alternatives, and consequences? |
| Planning | What do we intend to do, in what order, and what remains uncertain? |
| Active specs | What approved behaviour should be built or maintained? Mark implementation status separately. |
| Proposed specs | What behaviour is under consideration, not yet approved? |
| Recipes/runbooks | How do we perform or recover an operation safely? |
| Knowledge | What have we learned, with what evidence and applicability limits? |
| Audit | What did a scoped governance review discover, at which revision? |
| Archive | What is retained for history rather than current operational authority? |
| Agent context/index | Which authoritative links and constraints should an agent read first? |

## Compatibility example

A repository may use versioned documentation rather than this layout. If its
instructions declare `docs/v2` current, `docs/v1` historical, and `docs/v3`
future-only, record that mapping. Do not decide that the largest version number
is authoritative, or move all three directories into the preferred tree.

An active architecture page in `docs/v2` can remain there. A future proposal in
`docs/v3` remains proposed, not implemented. Historical decisions can retain their
original location with clear status and selective links. A navigation index may
solve discoverability without any file move.

## Optional migration procedure

Only when migration is authorised:

1. Inventory in-scope knowledge, current authority, inbound links, and tooling
   consumers. Respect all private and excluded paths.
2. Produce an old-to-new mapping with reasons, preservation destinations, risks,
   and validation. Prefer no move when a label or index is sufficient.
3. Obtain approval for the mapping and any merge, archive, or deletion. High
   confidence supports a proposal; it does not replace permission.
4. Create only needed destinations. Never overwrite a distinct existing document.
   Resolve collisions by comparing content and preserving unique knowledge.
5. Move or merge in small, reviewable steps. Update links, indexes, tooling paths,
   and explicit authority/lifecycle labels together.
6. Verify preserved knowledge, references, and tooling before retiring a source.
   Keep a move/merge record so history remains traceable.

Pure organisation must preserve all unique content. Removal of duplicated or
obsolete material requires a separate evidenced reconciliation action; it is not
an automatic consequence of migration. Do not leave contradictory live copies
merely to avoid deletion, and do not erase unique knowledge to produce a tidy tree.

## Completion

Report the content-role mapping, unchanged compatible areas, approved moves,
preserved knowledge, link checks, and remaining uncertainties. A repository with
no migration needed is a successful outcome.
