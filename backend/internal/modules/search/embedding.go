// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Embedder is the embed lane this module consumes; compose injects the
// ai router (or the offline fake) — search never picks a model.
type Embedder interface {
	Embed(ctx context.Context, req model.EmbedRequest) (model.Embeddings, error)
	// EmbedIdentity is the current binding's stamp — cheap, no API call.
	// identity = "<provider>/<model>@<dims>"; dims is the width guard's expected size.
	EmbedIdentity() (identity string, dims int)
}

// isZero reports whether every vector component is exactly 0. Cosine
// similarity against the zero vector is 0/0 = NaN, and a naive
// `ORDER BY sim DESC` sorts NaN FIRST — silently outranking every real
// match — so a zero vector must never reach storage.
func isZero(vec []float32) bool {
	for _, v := range vec {
		if v != 0 {
			return false
		}
	}
	return true
}

// UpsertEmbedding maintains one entity's vector. Content-hash keyed
// (ai-operational-spec §6): unchanged text under an unchanged embed
// binding costs NO model call — the returned bool says whether an
// embedding was actually computed. A text match under a CHANGED binding
// (an operator swap to a different provider/model/width) still
// re-embeds: skipping on hash alone would leave the row stamped with a
// model no longer serving the workspace, indistinguishable from a live
// one.
func (s *Store) UpsertEmbedding(ctx context.Context, entityType string, entityID ids.UUID, text string, embedder Embedder) (bool, error) {
	if !knownEntity(entityType) {
		return false, fmt.Errorf("search: unembeddable entity type %q", entityType)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return false, nil // nothing to embed; an empty vector helps nobody
	}
	sum := sha256.Sum256([]byte(text))
	hash := hex.EncodeToString(sum[:])
	identity, dims := embedder.EmbedIdentity()
	if identity == "" {
		// No embed lane bound (--ai-fake, or any routing config that never
		// declared an embeddings model) — a legitimate deployment shape
		// (brain.go's seedEmbedBinding carve-out), not an error. Embedding
		// is a no-op system-wide when unbound: returning here before the
		// stamp read skips both the DB round-trip and the width guard
		// below, which would otherwise fire on every call (dims stays 0,
		// but Embed's own zero-width default fills a live-width vector) and
		// keep EmbedGen.HandleEvent from ever acking, redelivering forever.
		return false, nil
	}

	stored, err := s.storedEmbeddingStamp(ctx, entityType, entityID)
	if err != nil {
		return false, err
	}
	next := embeddingStamp{hash: hash, model: identity}
	if stored == next {
		return false, nil // unchanged text, unchanged binding — never re-embed
	}

	// Between the read and the write, never inside a transaction: the call can
	// run to ai.CallCeiling, and an idle-in-transaction session that long holds a
	// pool slot, pins vacuum's horizon database-wide, and is killed by
	// database.IdleTransactionCeiling after the model call is already paid for.
	res, err := embedder.Embed(ctx, model.EmbedRequest{Inputs: []string{text}, Dimensions: dims})
	if err != nil {
		return false, fmt.Errorf("search: embed: %w", err)
	}
	if len(res.Vectors) != 1 || res.Dims != dims {
		return false, fmt.Errorf("search: embedder returned %d vectors of width %d, need 1×%d", len(res.Vectors), res.Dims, dims)
	}
	if isZero(res.Vectors[0]) {
		return false, fmt.Errorf("search: embedder returned a zero vector (cosine NaN)")
	}
	return s.writeEmbedding(ctx, entityType, entityID, stored, next, res.Vectors[0])
}

// embeddingStamp is what a stored vector was computed from: the text's hash
// and the binding's identity. Either one changing makes the vector stale.
type embeddingStamp struct {
	hash  string
	model string
}

// storedEmbeddingStamp reads the entity's stamp, the zero stamp when no row
// exists — the honest "nothing stored", distinct from a real empty hash or model.
func (s *Store) storedEmbeddingStamp(ctx context.Context, entityType string, entityID ids.UUID) (embeddingStamp, error) {
	var stored embeddingStamp
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT chunk_hash, model FROM embedding
			WHERE entity_type = $1 AND entity_id = $2 AND chunk_ix = 0`,
			entityType, entityID).Scan(&stored.hash, &stored.model)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return embeddingStamp{}, fmt.Errorf("search: reading the stored embedding stamp: %w", err)
	}
	return stored, nil
}

// writeEmbedding stores vec under next unless the row has moved past read since
// it was read, and reports whether it wrote.
func (s *Store) writeEmbedding(ctx context.Context, entityType string, entityID ids.UUID, read, next embeddingStamp, vec []float32) (bool, error) {
	fresh := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// CAS on the whole stamp read (zero when no row existed): a concurrent
		// writer that already moved the row past it — a redelivered event racing
		// this one, or a worker bound to another model re-embedding the same
		// text — already won, so leave fresh=false rather than clobber a row
		// fresher than the one this call started from. The hash alone misses
		// the second: a same-text swap leaves chunk_hash where it was.
		//
		// The activity arm re-checks the RESTRICTION and the AUDIENCE on write,
		// not only on the read that produced the text: a worker that read the
		// body just before an erasure held the row would otherwise reinsert a
		// vector of hidden content after the erasure deleted it
		// (A165/ADR-0114), and one that read it just before a human or a
		// verdict limited the audience would reinsert it after the narrowing
		// deleted it. Both windows are a model call wide.
		//
		// FOR SHARE, not the predicate alone. Read-committed evaluates the
		// subquery against a snapshot taken when the statement begins, so a
		// narrowing committing a moment later is invisible to it — and the
		// retraction that follows the narrowing would then DELETE a vector this
		// statement has not yet inserted, leaving the row indexed. The share
		// lock makes the two order: the retraction takes FOR UPDATE on the same
		// row, so either it waits for this insert and then deletes it, or this
		// insert waits and re-evaluates against the narrowed row.
		if entityType == entityActivity {
			var locked ids.UUID
			if err := tx.QueryRow(ctx,
				`SELECT id FROM activity WHERE id = $1 FOR SHARE`, entityID).Scan(&locked); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return nil // erased since the text was read; nothing to index
				}
				return fmt.Errorf("search: locking the activity being embedded: %w", err)
			}
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO embedding (entity_type, entity_id, chunk_ix, chunk_hash, model, embedding)
			SELECT $1, $2, 0, $3, $4, $5::vector
			 WHERE NOT EXISTS (SELECT 1 FROM activity a WHERE $1 = 'activity' AND a.id = $2
			                     AND (a.restricted_at IS NOT NULL OR a.audience <> 'workspace'))
			ON CONFLICT (entity_type, entity_id, chunk_ix)
			DO UPDATE SET chunk_hash = EXCLUDED.chunk_hash, model = EXCLUDED.model,
			              embedding = EXCLUDED.embedding, created_at = now()
			WHERE embedding.chunk_hash IS NOT DISTINCT FROM $6 AND embedding.model IS NOT DISTINCT FROM $7`,
			entityType, entityID, next.hash, next.model, vectorLiteral(vec), read.hash, read.model)
		if err != nil {
			return fmt.Errorf("search: upsert embedding: %w", err)
		}
		fresh = tag.RowsAffected() > 0
		return nil
	})
	return fresh, err
}

// VectorHit is one similarity result, already visibility-filtered.
type VectorHit struct {
	Type       string
	ID         ids.UUID
	Title      string
	Similarity float64
}

// SimilarEntities ranks entities by cosine similarity to the query
// vector, restricted to rows stamped with the caller's own embed
// identity. Object RBAC and row scope gate every branch, exactly like
// the lexical union — a vector hit is a read too.
func (s *Store) SimilarEntities(ctx context.Context, queryVec []float32, identity string, limit int, types ...string) ([]VectorHit, error) {
	return s.similarEntitiesWithin(ctx, queryVec, identity, limit, nil, types...)
}

// similarEntitiesWithin is SimilarEntities bounded to the records in within, with
// withinClause's reading of nil and of an empty set.
func (s *Store) similarEntitiesWithin(ctx context.Context, queryVec []float32, identity string, limit int, within []ids.UUID, types ...string) ([]VectorHit, error) {
	// A zero query vector makes every cosine distance 0/0 = NaN, and a
	// naive ORDER BY sim DESC sorts NaN FIRST — the same trap the write
	// path guards. There is nothing to rank against it, so return no vector
	// hits and let the caller's lexical arm carry the query.
	if isZero(queryVec) {
		return nil, nil
	}
	limit = clampLimit(limit)
	var hits []VectorHit
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		vecPos := arg(vectorLiteral(queryVec))
		// The e.model = $identity predicate is load-bearing for BOTH correctness (old-space rows must not
		// rank against new-space queries) AND crash-avoidance: the column is unbounded, so a bare
		// e.embedding <=> $q against a different-width row raises "different vector dimensions". The filter
		// excludes those rows before the projection computes <=>. NEVER remove it. (see design §5.6)
		identityPos := arg(identity)

		branches, err := similarBranchSQL(ctx, types, vecPos, identityPos, arg)
		if err != nil {
			return err
		}
		if len(branches) == 0 {
			return nil
		}
		sql := "SELECT rtype, id, title, sim FROM (" + strings.Join(branches, " UNION ALL ") + ") ranked"
		if bound := withinClause(within, arg); bound != "" {
			sql += " WHERE " + bound
		}
		sql += fmt.Sprintf(" ORDER BY sim DESC, rtype, id LIMIT $%d", arg(limit))
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return fmt.Errorf("search: similarity query: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var h VectorHit
			var title *string
			if err := rows.Scan(&h.Type, &h.ID, &title, &h.Similarity); err != nil {
				return err
			}
			if title != nil {
				h.Title = *title
			}
			hits = append(hits, h)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return hits, nil
}

// similarBranchSQL builds one ranked SELECT per requested-and-admitted record
// type — the vector arm's counterpart to admittedBranchSQL, and subject to the
// same two gates: object RBAC hides a denied type silently, then the branch
// carries the caller's scope clause. A vector hit is a read too.
//
// An empty types narrows nothing, which is what the retrieval seam asks for.
func similarBranchSQL(ctx context.Context, types []string, vecPos, identityPos int, arg func(any) int) ([]string, error) {
	var branches []string
	for _, branch := range searchBranches {
		// A word has no prose to embed, so a similarity search over one would
		// compare a name against a vector nothing produced.
		if branch.textOnly {
			continue
		}
		if len(types) > 0 && !slices.Contains(types, branch.entity) {
			continue
		}
		scope, admitted, err := branchScope(ctx, branch, "t", arg)
		if err != nil {
			return nil, err
		}
		if !admitted {
			continue
		}
		sql := fmt.Sprintf(
			`SELECT '%s'::text AS rtype, e.entity_id AS id, %s AS title,
			        (1 - (e.embedding <=> $%d::vector))::float8 AS sim
			 FROM embedding e JOIN %s t ON t.id = e.entity_id
			 WHERE e.entity_type = '%s' AND t.archived_at IS NULL AND e.model = $%d`,
			branch.entity, branch.title, vecPos, branch.table, branch.entity, identityPos,
		)
		if narrowing := branch.narrowing("t"); narrowing != "" {
			sql += " AND " + narrowing
		}
		if scope != "" {
			sql += " AND " + scope
		}
		branches = append(branches, sql)
	}
	return branches, nil
}

// vectorLiteral renders pgvector's input syntax; parameterized as text
// and cast, so no vector codec dependency rides the driver.
func vectorLiteral(vec []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
