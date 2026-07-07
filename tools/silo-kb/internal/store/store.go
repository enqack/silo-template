// Package store owns the Postgres side of the derived index: connection,
// idempotent schema migration, and the delta reindex algorithm.
package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
	"github.com/pgvector/pgvector-go"

	"silo.local/silo-kb/internal/chunk"
	"silo.local/silo-kb/internal/vault"
)

//go:embed schema.sql
var schemaSQL string

// DSN resolves the connection string: $SILOKB_DSN wins; otherwise self-locate
// by walking up from cwd to a .pg-data dir (matches the flake's pg-start
// layout). Self-location keeps .mcp.json and hooks free of env-expansion
// fragility.
func DSN() (string, error) {
	if dsn := os.Getenv("SILOKB_DSN"); dsn != "" {
		return dsn, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		pgdata := filepath.Join(dir, ".pg-data")
		if st, err := os.Stat(pgdata); err == nil && st.IsDir() {
			sock := pgdata
			// Mirror the flake's macOS sun_path-length fallback.
			if len(sock) > 90 {
				tmp := os.Getenv("TMPDIR")
				if tmp == "" {
					tmp = "/tmp"
				}
				sock = filepath.Join(tmp, fmt.Sprintf("silokb-%x", hashString(dir)))
			}
			return "postgres:///silokb?host=" + url.QueryEscape(sock) + "&port=5433", nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("SILOKB_DSN unset and no .pg-data found above cwd — run pg-start first")
		}
		dir = parent
	}
}

func hashString(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	dsn, err := DSN()
	if err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvec.RegisterTypes(ctx, conn)
	}
	return pgxpool.NewWithConfig(ctx, cfg)
}

// Migrate applies the embedded schema idempotently.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, schemaSQL)
	return err
}

// Embedder is satisfied by embed.Client; injected so reindex is testable.
type Embedder interface {
	Warmup(ctx context.Context) error
	Documents(ctx context.Context, texts []string) ([][]float32, error)
}

type ReindexStats struct {
	Notes         int
	SkippedNotes  int
	MovedNotes    int
	ChunksKept    int
	ChunksEmbedded int
	NotesPruned   int64
}

// Reindex delta-syncs the vault into Postgres in a single transaction: an
// embedding failure aborts the whole run — a partially embedded index is
// worse than a stale one, and hashes make reruns cheap.
func Reindex(ctx context.Context, pool *pgxpool.Pool, emb Embedder, notes []*vault.Note, full bool) (*ReindexStats, error) {
	if err := Migrate(ctx, pool); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if full {
		if _, err := tx.Exec(ctx, "truncate chunks, notes"); err != nil {
			return nil, err
		}
	}

	stats := &ReindexStats{}
	warmed := false
	seen := make([]uuid.UUID, 0, len(notes))

	for _, n := range notes {
		id, err := uuid.Parse(n.ID())
		if err != nil {
			return nil, fmt.Errorf("%s: bad id: %w", n.Path, err)
		}
		seen = append(seen, id)
		stats.Notes++

		var oldHash, oldPath string
		err = tx.QueryRow(ctx, "select content_hash, path from notes where id=$1", id).Scan(&oldHash, &oldPath)
		exists := err == nil
		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}

		fmJSON, err := json.Marshal(n.Frontmatter)
		if err != nil {
			return nil, fmt.Errorf("%s: frontmatter to json: %w", n.Path, err)
		}

		if exists && oldHash == n.ContentHash {
			// Body unchanged: chunks stay. Refresh metadata (covers moves —
			// graduation/archival — and frontmatter-only edits) without
			// touching embeddings.
			if oldPath != n.Path {
				stats.MovedNotes++
			} else {
				stats.SkippedNotes++
			}
			if _, err := tx.Exec(ctx,
				`update notes set path=$2, project=$3, type=$4, frontmatter=$5, updated_at=now() where id=$1`,
				id, n.Path, n.Project, n.Type(), fmJSON); err != nil {
				return nil, err
			}
			continue
		}

		if _, err := tx.Exec(ctx, `
			insert into notes (id, project, path, type, frontmatter, content_hash, updated_at)
			values ($1,$2,$3,$4,$5,$6,now())
			on conflict (id) do update set project=excluded.project, path=excluded.path,
				type=excluded.type, frontmatter=excluded.frontmatter,
				content_hash=excluded.content_hash, updated_at=now()`,
			id, n.Project, n.Path, n.Type(), fmJSON, n.ContentHash); err != nil {
			return nil, err
		}

		newChunks := chunk.Split(n)

		// Existing chunk hashes by ordinal — the delta diff.
		oldChunks := map[int]string{}
		rows, err := tx.Query(ctx, "select ordinal, content_hash from chunks where note_id=$1", id)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var ord int
			var h string
			if err := rows.Scan(&ord, &h); err != nil {
				rows.Close()
				return nil, err
			}
			oldChunks[ord] = h
		}
		rows.Close()

		var toEmbed []chunk.Chunk
		for _, c := range newChunks {
			if oldChunks[c.Ordinal] == c.Hash {
				stats.ChunksKept++
				continue
			}
			toEmbed = append(toEmbed, c)
		}

		// Drop ordinals that changed or fell off the end.
		if _, err := tx.Exec(ctx,
			"delete from chunks where note_id=$1 and ordinal >= $2", id, len(newChunks)); err != nil {
			return nil, err
		}
		if len(toEmbed) > 0 {
			if !warmed {
				if err := emb.Warmup(ctx); err != nil {
					return nil, fmt.Errorf("embedder warmup: %w", err)
				}
				warmed = true
			}
			texts := make([]string, len(toEmbed))
			for i, c := range toEmbed {
				texts[i] = c.Content
			}
			vecs, err := emb.Documents(ctx, texts)
			if err != nil {
				return nil, fmt.Errorf("%s: embed: %w", n.Path, err)
			}
			for i, c := range toEmbed {
				if _, err := tx.Exec(ctx, `
					insert into chunks (note_id, ordinal, heading_path, content, content_hash, embedding)
					values ($1,$2,nullif($3,''),$4,$5,$6)
					on conflict (note_id, ordinal) do update set heading_path=excluded.heading_path,
						content=excluded.content, content_hash=excluded.content_hash,
						embedding=excluded.embedding`,
					id, c.Ordinal, c.HeadingPath, c.Content, c.Hash, pgvector.NewVector(vecs[i])); err != nil {
					return nil, err
				}
				stats.ChunksEmbedded++
			}
		}
	}

	// Prune notes deleted from the vault (cascade removes their chunks).
	tag, err := tx.Exec(ctx, "delete from notes where not (id = any($1))", seen)
	if err != nil {
		return nil, err
	}
	stats.NotesPruned = tag.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return stats, nil
}
