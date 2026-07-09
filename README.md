# silo-template

A **silo** is a self-contained coordination center for independent agentic projects — not a
single codebase, but a workspace where a human operator and AI agents (Claude Code) share
long-lived context: projects, working theories, settled decisions, and the tooling to search
all of it. This repository is the template; clone or copy it to start a new silo, run
`nix develop`, and everything bootstraps itself.

## What lives here

```
silo-template/
├── knowledge-base/        # the knowledge vault — the single source of truth (markdown/OKF/Obsidian)
│   ├── daily/             # raw capture: append-only daily session logs
│   ├── deep-thoughts/     # raw capture: one timestamped reflection per file
│   ├── knowledge/         # working theory: mutable articles with confidence that decays
│   └── projects/          # asserted canon: durable per-project documentation, never decays
├── tools/silo-kb/         # Go tooling: derived Postgres index, lifecycle engine, MCP server
├── workspace/             # scratch area for project checkouts and working files
├── flake.nix              # Nix dev environment: auto-bootstraps and auto-starts the silo
├── CLAUDE.md              # the operating contract for AI agents
└── HUMAN.md               # the same contract, summarized for the human operator
```

## The core idea

Knowledge here has a **lifecycle**, and the design enforces an honest split:

- **Working theories** (`knowledge-base/knowledge/`) carry a confidence score that is
  reinforced when re-confirmed and decays when ignored. Theories that fade to zero are
  archived; theories that prove stable **graduate** into project canon.
- **Asserted canon** (`knowledge-base/projects/`) is settled documentation. It never decays
  and is never touched by the lifecycle — if it's there, it's asserted, not hypothesized.
- **Raw capture** (`daily/`, `deep-thoughts/`) is the provenance trail both tiers cite.

The markdown tree is the **single source of truth**. Postgres (pgvector + full-text) is a
derived search index — droppable at any time (`pg-nuke`) and rebuilt from the working tree in
one command. If the database ever disagrees with the markdown, the database is wrong.

## Retrieval

Every note is chunked, embedded locally (Ollama, `nomic-embed-text:v1.5`), and indexed for hybrid
search: semantic similarity and keyword rank fused with reciprocal rank fusion (equal-weighted).
Agents reach it through an MCP tool (`query_knowledge`); humans through `silo-kb query "…"`.

## Quickstart

```sh
nix develop        # bootstraps everything: git, vault, build, Postgres, model, index
```

The shell banner shows live status (✓/✗) and the full command reference; reprint it any time
with `silo-help`. The only requirement is Nix (flakes enabled) — the dev shell bundles everything
else, including [Ollama](https://ollama.com) for embeddings. If you already run a native Ollama (the
macOS app for Metal, or a NixOS `services.ollama`), the bootstrap reuses it instead of the bundled
server. Day-to-day operation is documented in [HUMAN.md](HUMAN.md); the agent-facing contract is
[CLAUDE.md](CLAUDE.md).

## Starting a new silo from this template

1. Copy or clone this repository under a new name.
2. `nix develop` — `silo-init` scaffolds anything missing and starts the stack.
3. Register your projects in the table in `CLAUDE.md` and add
   `knowledge-base/projects/<name>/` notes as they earn documentation.
