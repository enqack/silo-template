---
id: f14690fc-adf2-4f6c-baa4-a6c5515fc0a4
type: overview
title: silo-kb overview
description: Knowledge-base tooling for this silo — derived Postgres index, decay engine, MCP server
resource: tools/silo-kb
tags:
  - knowledge-base
  - tooling
timestamp: 2026-07-07 09:30:00
---

# silo-kb

Go tooling that maintains the derived Postgres index over `knowledge-base/` and runs
the knowledge lifecycle. One binary, subcommands:

| Command | Purpose |
|---|---|
| `silo-kb migrate` | apply schema idempotently |
| `silo-kb reindex [--full]` | delta reindex the vault into Postgres |
| `silo-kb query "text"` | hybrid RRF retrieval (pgvector + tsvector) |
| `silo-kb compile` | reinforce/decay/archive/graduate knowledge articles |
| `silo-kb sync-index` | regenerate the generated knowledge index |
| `silo-kb inject-index` | emit tier-priority-truncated index for session start |
| `silo-kb validate` | frontmatter contract validation (also the PreToolUse hook) |
| `silo-kb serve-mcp` | stdio MCP server exposing query_knowledge |

Source: [tools/silo-kb](../../../tools/silo-kb).
