// Package query implements hybrid retrieval: cosine similarity over pgvector
// fused with ts_rank over native tsvector via reciprocal rank fusion, in one
// SQL round trip. Always both legs — no mode switch.
package query

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

const (
	rrfK          = 60 // standard RRF constant
	candidatePool = 50 // per-leg candidates before fusion
	DefaultTopK   = 8
)

type Result struct {
	Path        string
	Type        string
	HeadingPath string
	Content     string
	Score       float64
}

const sql = `
with vec as (
  select c.id, row_number() over (order by c.embedding <=> $1) as r
  from chunks c join notes n on n.id = c.note_id
  where ($3 = '' or n.project = $3) and c.embedding is not null
  order by c.embedding <=> $1
  limit $5
), kw as (
  select c.id, row_number() over (order by ts_rank_cd(c.fts, q) desc) as r
  from chunks c
  join notes n on n.id = c.note_id,
  websearch_to_tsquery('english', $2) q
  where c.fts @@ q and ($3 = '' or n.project = $3)
  limit $5
)
select n.path, n.type, coalesce(c.heading_path, ''), c.content,
       coalesce(1.0/($6 + vec.r), 0) + coalesce(1.0/($6 + kw.r), 0) as score
from chunks c
join notes n on n.id = c.note_id
left join vec on vec.id = c.id
left join kw on kw.id = c.id
where vec.id is not null or kw.id is not null
order by score desc
limit $4`

// Run fuses both retrieval legs. queryVec must be embedded with the
// search_query task prefix (embed.Client.Query does this).
func Run(ctx context.Context, pool *pgxpool.Pool, queryVec []float32, queryText, project string, topK int) ([]Result, error) {
	if topK <= 0 {
		topK = DefaultTopK
	}
	rows, err := pool.Query(ctx, sql,
		pgvector.NewVector(queryVec), queryText, project, topK, candidatePool, rrfK)
	if err != nil {
		return nil, fmt.Errorf("rrf query: %w", err)
	}
	defer rows.Close()

	var out []Result
	for rows.Next() {
		var r Result
		if err := rows.Scan(&r.Path, &r.Type, &r.HeadingPath, &r.Content, &r.Score); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
