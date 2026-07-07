# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this silo.

## Silo Structure                                                     
                                                                            
This is a Multi-Project Silo - a coordination center for independent agentic projects, not a single codebase.   

### Projects

| Project | Location | Description | Remote Repository (Git) |
|---------|----------|-------------|-------------------------|
| silo-kb | `tools/silo-kb/` | Knowledge-base tooling: derived Postgres index, decay engine, MCP server | — |


## Knowledge base

`knowledge-base/` is an [Open Knowledge Format (OKF) v0.1](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md)
bundle, also usable directly as an Obsidian vault. Entry point:
[knowledge-base/index.md](knowledge-base/index.md). Conventions (portable — apply the same rules in
any silo's knowledge base):

OKF was chosen deliberately: it's a published, tool-agnostic standard (plain markdown + YAML
frontmatter + wikilinks), so the vault stays portable — it opens as-is in Obsidian and moves to
Logseq/Roam without a migration — and the frontmatter contract below is enforceable independently of
any single editor. Nothing here is load-bearing on OKF-specific machinery; the format is a convention,
not a dependency.

- **Reserved filenames:** `index.md` and `log.md` are exempt from the `type` frontmatter requirement.
  Only the root `knowledge-base/index.md` may carry frontmatter, and only `okf_version: "0.1"` — every
  other `index.md`/`log.md` must have no frontmatter at all.
- **Every other `.md` file** needs YAML frontmatter with a required `type` field. OKF doesn't mandate a
  fixed taxonomy — pick a small, consistent set of `type` values that fits this silo's content and
  reuse them across notes rather than inventing a new one per note. Recommended fields, in order:
  `title`, `description`, `resource` (a repo-relative path), `tags`, `timestamp`. The `type` values in
  use in this silo (extend deliberately, don't invent per-note):

  | `type` | Tier | Meaning |
  |---|---|---|
  | `daily-log` | `daily/` | append-only capture of a day's work |
  | `deep-thought` | `deep-thoughts/` | a single standalone reflection |
  | `concept` | `knowledge/` | a reusable pattern/principle/technique (working theory) |
  | `overview` | `projects/` | a project-level canon note |

- **`knowledge/` subdirectory layout** (organizational only — the generated index groups by `type`,
  not by directory; add a subdirectory when it earns its keep, not speculatively):
  - `concepts/` — reusable patterns, principles, and techniques (working-theory articles).
  - `cursed-knowledge/` — surprising gotchas, i.e. things you wish you didn't have to know. This is
    the durable home for the `## Cursed Knowledge` entries the session-end extractor
    (`.claude/hooks/session-end-extract.sh`) collects into daily logs; graduate the ones worth
    keeping into an article here.
  - `lessons-learned/` — postmortem reflections: what worked, what didn't, and why. Authored
    deliberately (nothing auto-populates it), and worth keeping even when empty.
  - `archive/` — retired notes: `archive/faded/` (decayed to zero confidence) and
    `archive/falsified/` (judged false).
- **Links:** internal links use Obsidian wikilinks (`[[note]]`); citations to actual repo files use
  plain relative markdown links instead.
- **Adding a new project:** create `knowledge-base/projects/<project-name>/` following the same shape
  as existing projects in this vault — flat project-level notes (overview, build tooling, testing,
  conventions, etc.), an `architecture/` subdirectory (one note per major subsystem/component), and a
  `concepts/` subdirectory for cross-cutting patterns referenced from multiple notes.

### Two-tier vault + frontmatter contract

The vault has two knowledge tiers plus two raw-capture tiers. The markdown tree is the single
source of truth; Postgres is a derived, droppable index (`silo-kb reindex --full` rebuilds it).

- `daily/YYYY-MM-DD.md` (`type: daily-log`, append-only) and `deep-thoughts/` (`type: deep-thought`)
  are raw capture. They never graduate directly; `knowledge/` articles cite them via `sources:`.
- `knowledge/**` is **working theory** — mutable, confidence-decaying. Required frontmatter beyond
  `id`/`type`: `confidence` (0–1), `maturity` (`seed`|`developing`|`stable`), `last_reinforced`,
  `reinforce_count`, `sources` (non-empty). Optional `status`: `active` (default) or `disputed` — a
  note the agent contests but hasn't disproven stays live and flagged, so dissent is recorded rather
  than lost to passive decay. (A note judged outright false is falsified instead — see below.)
- `projects/**` is **asserted canon** — durable, exempt from decay. Must NOT carry decay fields.
- Every non-`index.md`/`log.md` note carries a stable `id` (UUID, assigned once — UUIDv7 for new
  notes — never reassigned). Moves (graduation, archival) keep the `id`; the index keys on it.
- `knowledge/index.md` is GENERATED (`silo-kb sync-index`) — never hand-edit it.
- `knowledge/log.md` is the compilation audit trail — appended by `silo-kb compile` only.
- Timestamps are formatted `YYYY-MM-DD HH:MM:SS` (local time) everywhere they appear.

Lifecycle (run via `/kb-compile`): reinforcement +0.1 (explicit, agent-justified), decay −0.1 when
stale >30 days, confidence ≤ 0 → `knowledge/archive/faded/`, git-age >6 months → `knowledge/archive/`,
and stable articles graduate — move, not copy — into `projects/<project>/`, dropping decay fields.
Falsification (`--falsify <id>=<reason>`, explicit and agent-justified) is a separate, active path: a
theory judged false is moved to `knowledge/archive/falsified/` with `status: falsified` and its reason
recorded — it wins over reinforce/decay, so being wrong is distinguished from being forgotten.

### Knowledge tooling (silo-kb)

Dev environment: `nix develop` auto-starts the silo — the shellHook runs `silo-init` (idempotent
full bootstrap: git init, vault scaffold for a fresh silo, Go build, Postgres via `pg-start`,
Ollama model check/pull, reindex + sync-index) and then prints a status/command banner via
`silo-help`. Set `SILOKB_NO_AUTOSTART=1` to skip the bootstrap; `pg-start`/`pg-stop`/`pg-nuke`
manage Postgres directly; DSN is exported as `SILOKB_DSN`. Embeddings come from local Ollama
(`nomic-embed-text:v1.5`, version-pinned). Changing the embedding model (or its tag) requires a full
reindex — `silo-kb reindex --full` — since old and new embeddings aren't comparable. Manual build:
`cd tools/silo-kb && go build -o silo-kb .`

| Command | Purpose |
|---|---|
| `silo-kb reindex [--full]` | delta-sync the vault into Postgres (chunks + embeddings) |
| `silo-kb query "text" [--project P] [--top-k N]` | hybrid RRF retrieval |
| `silo-kb compile [--dry-run] [--reinforce …] [--falsify …] [--graduate …]` | knowledge lifecycle run |
| `silo-kb sync-index` | regenerate `knowledge-base/knowledge/index.md` |
| `silo-kb inject-index --budget N` | truncated index for SessionStart injection |
| `silo-kb serve-mcp` | stdio MCP server (`query_knowledge`) |

Slash commands: `/kb-reindex`, `/kb-query`, `/kb-compile`, `/kb-sync-index`.
