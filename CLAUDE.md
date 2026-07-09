# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this silo. It is
the agent-facing operating contract; [HUMAN.md](HUMAN.md) is the operator's digest of the same system
and [README.md](README.md) is the newcomer's orientation. All three share the section order below.

## What this is

This is a **Multi-Project Silo** — a coordination center for independent agentic projects, not a single
codebase. A human operator and AI agents (you) share long-lived context here: the projects the silo
coordinates, working theories, settled decisions, and the tooling to search all of it.

The projects coordinated in this silo are registered in [PROJECTS.md](PROJECTS.md) — consult it for the
current list, locations, and remotes, and add a row there when onboarding a project. (`silo-kb reset`
restores `PROJECTS.md` to its empty template along with the vault.)

## Layout & tiers

`knowledge-base/` is the knowledge vault and the **single source of truth**. It is an
[Open Knowledge Format (OKF) v0.1](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md)
bundle, also usable directly as an Obsidian vault. Entry point:
[knowledge-base/index.md](knowledge-base/index.md).

```
knowledge-base/
├── daily/          # raw capture: append-only daily session logs         (type: daily-log)
├── deep-thoughts/  # raw capture: one timestamped reflection per file     (type: deep-thought)
├── knowledge/      # working theory: mutable articles, confidence decays  (type: concept, …)
└── projects/       # asserted canon: durable per-project docs, no decay   (type: overview, …)
```

The vault has **two knowledge tiers plus two raw-capture tiers**:

- **Raw capture** — `daily/YYYY-MM-DD.md` (append-only) and `deep-thoughts/`. These are the provenance
  trail. They never graduate directly; `knowledge/` articles cite them via `sources:`.
- **Working theory** — `knowledge/**` is mutable and confidence-decaying: hypotheses that are
  reinforced when re-confirmed and decay when ignored.
- **Asserted canon** — `projects/**` is durable, settled documentation, exempt from the lifecycle. If a
  note lives here, it's asserted, not hypothesized.

The signal is the directory. The markdown tree is authoritative; Postgres (pgvector + full-text) is a
**derived, droppable index** — `silo-kb reindex --full` rebuilds it entirely from the working tree. If
the database ever disagrees with the markdown, the database is wrong.

**Why OKF:** it's a published, tool-agnostic standard (plain markdown + YAML frontmatter + wikilinks),
so the vault stays portable — it opens as-is in Obsidian and moves to Logseq/Roam without a migration —
and the frontmatter contract below is enforceable independently of any single editor. Nothing here is
load-bearing on OKF-specific machinery; the format is a convention, not a dependency.

**`type` taxonomy** (extend deliberately, don't invent a new value per note):

| `type` | Tier | Meaning |
|---|---|---|
| `daily-log` | `daily/` | append-only capture of a day's work |
| `deep-thought` | `deep-thoughts/` | a single standalone reflection |
| `concept` | `knowledge/` | a reusable pattern/principle/technique (working theory) |
| `overview` | `projects/` | a project-level canon note |

**`knowledge/` subdirectory layout** (organizational only — the generated index groups by `type`, not
by directory; add a subdirectory when it earns its keep, not speculatively):

- `concepts/` — reusable patterns, principles, and techniques (working-theory articles).
- `cursed-knowledge/` — surprising gotchas, i.e. things you wish you didn't have to know. The durable
  home for the `### Cursed Knowledge` entries the session-end extractor
  (`.claude/hooks/session-end-extract.sh`) collects into daily logs; graduate the ones worth keeping
  into an article here.
- `lessons-learned/` — postmortem reflections: what worked, what didn't, and why. Authored
  deliberately (nothing auto-populates it), and worth keeping even when empty.
- `archive/` — retired notes: `archive/faded/` (decayed to zero confidence) and `archive/falsified/`
  (judged false).

## The lifecycle

Run via `/kb-compile` (or `silo-kb compile` by hand). Each run:

- **Reinforces** articles the agent explicitly justifies: +0.1 confidence (capped at 1.0). Mere
  mention doesn't count; nothing is inferred. Reinforcement is also the only thing that promotes
  **maturity**: `seed`→`developing` at confidence ≥0.8; `developing`→`stable` at ≥0.9 with
  `reinforce_count` ≥3. Promotion never happens on decay or by hand-editing.
- **Decays** articles stale >30 days: −0.1 per run.
- **Falsifies** on demand (`--falsify <id>=<reason>`, explicit and agent-justified): a theory judged
  outright false moves to `knowledge/archive/falsified/` with `status: falsified` and its reason
  recorded. This wins over reinforce/decay — being wrong is distinguished from being forgotten. (For a
  note you contest but haven't disproven, set `status: disputed` and leave it live.)
- **Archives** faded articles (confidence ≤ 0 → `knowledge/archive/faded/`) and ancient ones (no git
  commit in >6 months → `knowledge/archive/`). A note reinforced in the same run is shielded from
  ancient-archival.
- **Graduates** on demand (`--graduate <id>:projects/<project>/<note>.md`, explicit and agent-justified
  like reinforcement; only `stable` notes qualify): the article **moves — not copies** — into canon,
  dropping its decay fields and keeping provenance (`sources:`). The destination must be inside a
  project subdirectory — the indexer never sees notes directly under `projects/`.

`knowledge/log.md` is the compilation audit trail, appended by `silo-kb compile` only. `git log` plus
that file explains why any note changed, moved, or vanished.

## The frontmatter contract

Enforced by `silo-kb validate` (the PreToolUse hook inside Claude Code and the pre-commit gate).

- **Reserved filenames:** `index.md` and `log.md` are exempt from the `type` requirement. Only the root
  `knowledge-base/index.md` may carry frontmatter, and only `okf_version: "0.1"` — every other
  `index.md`/`log.md` must have no frontmatter at all.
- **Every other `.md` note** needs YAML frontmatter with a required `type` field and a stable `id`
  (UUID, assigned once — UUIDv7 for new notes — never reassigned; moves keep the `id`, and the index
  keys on it). Recommended fields, in order: `title`, `description`, `resource` (a repo-relative path),
  `tags`, `timestamp`.
- **`knowledge/**` (working theory)** additionally requires: `confidence` (0–1), `maturity`
  (`seed`|`developing`|`stable`), `last_reinforced`, `reinforce_count`, and a non-empty `sources` list.
  Optional `status`: `active` (default) or `disputed`.
- **`projects/**` (asserted canon)** must **NOT** carry any decay fields.
- **Timestamps** are `YYYY-MM-DD HH:MM:SS` (local time) everywhere a full timestamp appears — except
  daily-log capture-batch headings, which are time-only `## HH:MM:SS` (the file is already dated by its
  frontmatter and `# <date>` h1).
- **Links:** internal vault links use Obsidian wikilinks (`[[note]]`); citations to actual repo files
  use plain relative markdown links.
- **Never hand-edit** `knowledge/index.md` (GENERATED by `silo-kb sync-index`) or `knowledge/log.md`
  (appended by `silo-kb compile` only). The write-time hook blocks agents from editing these and from
  writing notes with broken frontmatter; a tracked git pre-commit hook (`.githooks/pre-commit`, wired
  by `silo-init` via `core.hooksPath`) runs the same validator so the contract holds outside Claude
  Code too.

## Tooling & commands

Dev environment: `nix develop` auto-starts the silo — the shellHook runs `silo-init` (idempotent full
bootstrap: git init, vault scaffold for a fresh silo, Go build, Postgres via `pg-start`, Ollama via
`ollama-start` + model pull, reindex + sync-index) and then prints a status/command banner via
`silo-help`. Set `SILOKB_NO_AUTOSTART=1` to skip the bootstrap; `pg-start`/`pg-stop`/`pg-nuke` manage
Postgres directly; `ollama-start`/`ollama-stop` manage the embedding server. Ollama is bundled
(`pkgs.ollama`) but portable: `ollama-start` reuses a server already listening on `:11434` — a native
macOS app (Metal) or a NixOS `services.ollama` — and only starts the bundled one as a fallback. DSN is
exported as `SILOKB_DSN`. Embeddings come from local Ollama (`nomic-embed-text:v1.5`, version-pinned);
changing the embedding model (or its tag) requires a full reindex — `silo-kb reindex --full` — since
old and new embeddings aren't comparable. Manual build: `cd tools/silo-kb && go build -o silo-kb .`

| Command | Purpose |
|---|---|
| `silo-kb reindex [--full]` | delta-sync the vault into Postgres (chunks + embeddings) |
| `silo-kb reset --force` | wipe the vault back to the fresh-silo scaffold, then rebuild the index (destructive; git is the recovery net) |
| `silo-kb query "text" [--project P] [--top-k N]` | hybrid RRF retrieval |
| `silo-kb compile [--dry-run] [--reinforce …] [--falsify …] [--graduate …]` | knowledge lifecycle run |
| `silo-kb sync-index` | regenerate `knowledge-base/knowledge/index.md` |
| `silo-kb inject-index --budget N` | truncated index for SessionStart injection |
| `silo-kb validate` | check the whole vault against the frontmatter contract (also the PreToolUse/pre-commit gate) |
| `silo-kb migrate` | apply the derived-index schema idempotently (reindex runs it automatically) |
| `silo-kb serve-mcp` | stdio MCP server (`query_knowledge`) |

Slash commands: `/kb-reindex`, `/kb-query`, `/kb-compile`, `/kb-sync-index`, `/kb-reset`. Retrieval is
also exposed to agents through the `query_knowledge` MCP tool.

## Automatic session behavior

The two raw-capture tiers are meant to be **written by the agent when a situation calls for one**, not
supplied by the operator. Don't wait to be asked.

- **Session start:** a truncated knowledge index is injected into context (knowledge/projects entries
  survive truncation first).
- **On writes:** the frontmatter validator hook (see The frontmatter contract) blocks violations with a
  correction message.
- **Daily log — capture in-session.** As durable material surfaces, append it to
  `knowledge-base/daily/YYYY-MM-DD.md` (today's date). The log is **time-primary**: each capture pass
  is its own `## HH:MM:SS` block (local time, from `date "+%H:%M:%S"` — time-only, the file is already
  dated), and the categories are `###` subsections *inside* that block, created only when you have
  entries for them: `### Concepts` (durable ideas/decisions), `### Cursed Knowledge` (surprising
  gotchas), `### Unresolved` (open questions as `- [ ]` checkboxes), `### Log` (one-line summaries of
  what was done). Categorized bullets only — no freeform prose. Append-only: reuse the current pass's
  `## <time>` block while you're still in that pass; a later capture moment appends a new `## <time>`
  block. If the file doesn't exist, create it with frontmatter — a fresh lowercase `uuidgen` `id`,
  `type: daily-log`, `title: <date>`, `timestamp: YYYY-MM-DD HH:MM:SS` — then an h1 `# <date>`, then
  the first `## <time>` block. This is raw capture — never graduate directly into `knowledge/` or
  `projects/` (that stays a deliberate `/kb-compile` act).
- **Deep thoughts — generate at notable moments (moderate cadence).** When a session hits a notable
  milestone or a vivid moment of friction or triumph, write a grounded Jack Handey–style reflection
  following the [Deep Thoughts](#deep-thoughts) appendix. Multiple per session are fine; don't force
  one when nothing notable happened. If the user-level `deep-thoughts` skill happens to be installed,
  its `/deep-thoughts` command still works — but the appendix is this silo's source of truth and
  requires no skill to be present.
- **Session end:** the `SessionEnd` hook (`session-end-extract.sh`) is a headless backstop that appends
  any still-missing categorized notes from the session to today's daily log (requires a one-time
  `claude /login`; writes only to `daily/`). In-session capture need not be exhaustive — capture what's
  clearly durable as you go and let the hook catch the rest.

## Adding a project

Register the project in the table in [PROJECTS.md](PROJECTS.md), then create
`knowledge-base/projects/<project-name>/` following the same shape as existing projects: flat
project-level notes (overview, build tooling, testing, conventions, etc.), an `architecture/`
subdirectory (one note per major subsystem/component), and a `concepts/` subdirectory for cross-cutting
patterns referenced from multiple notes.

---

## Deep Thoughts

Self-contained instructions for generating a Jack Handey–style "Deep Thought" — this silo does not
depend on the `deep-thoughts` skill being installed; everything needed is here.

### Voice & Style Guide

"Deep Thoughts by Jack Handey" is a recurring SNL segment. Reproduce this exact register:

- **Deadpan earnestness** — the narrator is never in on the joke; play it completely straight.
- **Narrator as subject** — the narrator is the butt of the joke; never punch outward at others.
- **Brevity** — 1 to 4 sentences maximum; the shorter, the better.
- **No winking** — no "haha", no "just kidding", no meta-commentary, no emoji.
- **Mock-philosophical gravity** — sound like you're imparting wisdom even as you reveal something
  uncomfortable.

**The core mechanism: comedic bait-and-switch.** Every Deep Thought runs on a strict contrast between
a gentle, poetic **setup** and a completely unhinged, coldly logical, or nakedly selfish
**conclusion**. The softer and more sincere the opening, the harder the turn lands. If the setup and
the payoff feel like they belong to the same sentence, you haven't turned hard enough.

**The three-part structure:**

1. **The Gentle Setup** — open on a cliché, a New Age observation, a childhood memory, or a classic
   philosophical premise. Use soft imagery: butterflies, grandparents, wishes, rivers, the stars.
2. **The Turn** — a sudden pivot that derails the pleasant thought, triggered by a mundane reality, a
   bizarre choice, or a dark twist. This is the hinge; it should feel abrupt, not eased into.
3. **The Deadpan Wrap-up** — a matter-of-fact conclusion that treats the absurdity as perfectly
   reasonable, as if it obviously followed from the setup.

**The persona (embody, don't describe):**

- **Unearned confidence** — he genuinely believes these thoughts are profound and helpful to humanity.
- **Casual cruelty** — he says terrible, selfish, or sociopathic things with total innocence and no
  malice; he does not notice they are terrible.
- **Childlike logic** — he resolves complex emotional or philosophical problems with absurd, literal,
  caveman-simple reasoning, and is satisfied by it.

Reference register (do not reuse these, just calibrate to them):

> "If you ever drop your keys into a river of molten lava, let 'em go, because man, they're gone."

> "Before you criticize someone, you should walk a mile in their shoes. That way, when you criticize
> them, you're a mile away and you have their shoes."

> "I hope if dogs ever take over the world and they choose a king, they don't just go by size,
> because I bet there are some Chihuahuas with some good ideas."

### Session Reflection (do this first, every time)

Before generating the thought, scan the current conversation for raw material:

1. **Identify notable events** — tasks completed, files created or edited, bugs encountered, tools
   used, decisions made, creative choices, recurring themes, moments of friction or triumph.
2. **Extract 2–3 concrete details** — be specific (e.g., "fixed frontmatter in three song files",
   "chose Phrygian Dominant over Dorian", "the lint script found 7 orphaned links").
3. **Distill a topic** — name the most prominent session thread in 1–3 words.
4. **Ground the thought** — the absurdist pivot should feel earned by the actual session events, not
   generic. The narrator is reflecting on something they literally just did or experienced.

The result should feel like: *"I just spent 40 minutes on this, and somehow the universe has
something to say about it."*

### Output location & format

Deep thoughts go to `knowledge-base/deep-thoughts/YYYY-MM-DD-HH-MM-{slug}.md` with OKF frontmatter
(`id` via `uuidgen`, `type: deep-thought`, `title`, `timestamp: YYYY-MM-DD HH:MM:SS`) matching the
existing notes there.

### Generation steps

1. Complete Session Reflection above.
2. Draft the thought(s) — run the Quality Check below mentally before committing.
3. Determine the topic slug: lowercase, spaces to hyphens, strip punctuation.
4. Get the current local timestamp with the Bash tool: `date "+%Y-%m-%d-%H-%M"` for the filename, and
   `date "+%Y-%m-%d %H:%M:%S"` for the timestamp shown in the frontmatter.
5. Build the output path: `knowledge-base/deep-thoughts/YYYY-MM-DD-HH-MM-{topic-slug}.md`.
6. Create the `knowledge-base/deep-thoughts/` directory if it does not exist (`mkdir -p`), and
   generate a fresh lowercase `id` with `uuidgen`.
7. Write the file using the Write tool (never use echo or heredoc), with OKF frontmatter followed by
   the thought as a blockquote.
8. Display the thought to the user in the response, followed by the file path where it was saved.

### Quality Check (before writing the file)

1. **Gentle Setup** — does it open on a soft, sincere, or poetic premise?
2. **The Turn** — is there a sharp pivot to something absurd, dark, or self-incriminating (not an
   easing-in, but a hinge)?
3. **Deadpan Wrap-up** — does it land the absurdity as if it were perfectly reasonable?
4. **Contrast** — is the gap between setup and conclusion wide? (If they feel like the same thought,
   turn harder.)
5. **Persona** — does the narrator show unearned confidence, casual cruelty, or childlike logic — and
   never notice?
6. Is it deadpan — no humor cues, no winking, no emoji?
7. Is it 1–4 sentences?
8. Is the narrator the subject of the joke, never punching outward?
9. Is it visibly connected to something that actually happened this session?

If any check fails, redraft before proceeding.
