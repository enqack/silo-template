---
id: de6d5441-c97e-415f-b5ca-0df850ff0d84
type: concept
title: Postgres as derived index
confidence: 0.7
maturity: seed
last_reinforced: 2026-07-07 09:00:00
reinforce_count: 0
sources:
  - "[[2026-07-07]]"
---

# Postgres as derived index

The OKF markdown tree is the single source of truth. Postgres (pgvector + native
tsvector) is a derived index: droppable and rebuildable from git HEAD in one command,
never authoritative. If the DB drifts from what's on disk, the design has failed.

## Rebuild guarantee

`pg-nuke && pg-start && silo-kb reindex --full` must reproduce the entire index from
the working tree. Nothing lives only in the database — embeddings, chunk boundaries,
and metadata are all recomputable from the markdown.

## Falsification

Chunk count exceeds ~200k, or a routine full reindex exceeds tolerable wall-clock
time. Either fires → revisit a split vector/metadata design.
