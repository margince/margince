// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// Giving a document's passages their vectors.
//
// This module owns its own vectors rather than joining search's `embedding`
// table — see the package doc for why — which means it owns the four
// invariants that come with a pgvector column. Three of them live in
// platform/vectorkit, spelled once for both owners; the fourth is the pairing
// CHECK in the schema, and the write below is shaped so it cannot be violated.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/vectorkit"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/ports/model"
)

// embedBatch is how many passages ride one model call. Batching is what makes
// a document of a thousand passages one round trip per hundred rather than a
// thousand; the number is small enough that a provider's per-request input
// ceiling is never the thing that decides it.
const embedBatch = 64

// pendingChunk is one passage the embed walk may have to compute.
type pendingChunk struct {
	id   ids.UUID
	text string
	hash string
	// storedIdentity is the binding the row's vector was computed under, empty
	// when the row has none yet. Both halves decide whether a call is owed:
	// see vectorkit.Unchanged.
	storedIdentity string
}

// EmbedDocument gives every one of a document's passages a current vector, and
// reports how many actually cost a model call.
//
// Unchanged text under an unchanged binding costs nothing, which is what makes
// the re-embed sweep resumable for free: it can be run again after a crash and
// the passages already done are skipped rather than recomputed.
//
// With no embed lane bound this is a no-op rather than an error. An
// installation configured without one is a legitimate deployment shape, and the
// ask answers `retrieval_unavailable` there — a distinct outcome from a corpus
// that simply does not cover the question.
func (s *Store) EmbedDocument(ctx context.Context, documentID ids.UUID, e vectorkit.Embedder) (int, error) {
	identity, dims := e.EmbedIdentity()
	if identity == "" {
		return 0, nil
	}
	pending, err := s.chunksNeedingEmbedding(ctx, documentID, identity)
	if err != nil {
		return 0, err
	}
	embedded := 0
	for start := 0; start < len(pending); start += embedBatch {
		end := min(start+embedBatch, len(pending))
		n, err := s.embedBatch(ctx, pending[start:end], e, identity, dims)
		if err != nil {
			return embedded, err
		}
		embedded += n
	}
	return embedded, nil
}

// chunksNeedingEmbedding reads the document's passages and drops the ones
// already current under this binding.
//
// The decision is made in Go rather than as a WHERE clause because
// vectorkit.Unchanged is the ONE place that decides it — the same function the
// drift sweep and the search side read. A predicate spelled here would be a
// second answer to the question, and the two would disagree the first time
// either moved.
func (s *Store) chunksNeedingEmbedding(ctx context.Context, documentID ids.UUID, identity string) ([]pendingChunk, error) {
	var pending []pendingChunk
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, text, chunk_hash, coalesce(embed_identity, '')
			 FROM knowledge_chunk
			 WHERE document_id = $1 AND archived_at IS NULL
			 ORDER BY chunk_ix`, documentID)
		if err != nil {
			return fmt.Errorf("read the document's passages: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var c pendingChunk
			if err := rows.Scan(&c.id, &c.text, &c.hash, &c.storedIdentity); err != nil {
				return err
			}
			// The hash is RECOMPUTED from the text this read returned, and
			// compared against the one stored beside it. Passing the stored
			// hash as both arguments made the comparison a tautology — it
			// reduced Unchanged to an identity check, so the half of it that
			// exists to notice moved TEXT was dead at this call site.
			//
			// The two normally agree, because the writer sets them together.
			// When they do not — a row edited outside the writer, a migration,
			// a defect — the row holds a vector of prose it no longer contains,
			// and re-embedding it is the only honest answer.
			if vectorkit.Unchanged(c.hash, c.storedIdentity, Chunk{Text: c.text}.Hash(), identity) {
				continue
			}
			pending = append(pending, c)
		}
		return rows.Err()
	})
	return pending, err
}

// embedBatch computes and stores one batch's vectors.
func (s *Store) embedBatch(ctx context.Context, batch []pendingChunk, e vectorkit.Embedder, identity string, dims int) (int, error) {
	inputs := make([]string, len(batch))
	for i, c := range batch {
		inputs[i] = c.text
	}
	res, err := e.Embed(ctx, model.EmbedRequest{Inputs: inputs, Dimensions: dims})
	if err != nil {
		return 0, fmt.Errorf("embed the document's passages: %w", err)
	}
	// Checked before anything is written, and checked as a COUNT as well as a
	// width: a provider that answered with fewer vectors than inputs would
	// otherwise have its answers written against the wrong passages, which is
	// a corpus that cites the wrong document rather than one that fails.
	if len(res.Vectors) != len(batch) || res.Dims != dims {
		return 0, fmt.Errorf("the embed lane returned %d vectors of width %d, need %d×%d",
			len(res.Vectors), res.Dims, len(batch), dims)
	}
	for i, vec := range res.Vectors {
		// A zero vector must reach neither storage nor a query: cosine against
		// it is 0/0 = NaN and `ORDER BY sim DESC` sorts NaN FIRST, so one
		// stored zero silently outranks every real passage in the corpus.
		if vectorkit.IsZero(vec) {
			return 0, fmt.Errorf("the embed lane returned a zero vector for passage %s (cosine NaN)", batch[i].id)
		}
	}
	return s.storeVectors(ctx, batch, res.Vectors, identity)
}

// storeVectors writes the batch's vectors and reports how many landed.
//
// The vector, its identity and its timestamp are written in ONE update per row.
// Written separately, a row would exist carrying an identity and no vector —
// retrievable by the ask's identity filter and unrankable when it got there,
// which is worse than an unembedded row and is what the pairing CHECK refuses.
//
// The write is a CAS on BOTH halves of what was read, and both are needed.
//
// The HASH: a re-ingest that rewrote this passage while the model call was in
// flight has already replaced the row's text, and stamping this vector onto it
// would claim a vector of prose that is no longer there.
//
// The IDENTITY: two passes can overlap under different bindings — a drift
// sweep repairing a corpus while an ingest embeds a new document, say — and the
// one that started EARLIER can finish later. Without this half it would
// overwrite the newer binding's vector with an older one, and the corpus that
// had just become answerable would report not_ready again, because the ask
// counts and retrieves only at the live identity. A row whose identity has
// moved since it was read is a row this pass no longer has anything to say
// about.
func (s *Store) storeVectors(ctx context.Context, batch []pendingChunk, vectors [][]float32, identity string) (int, error) {
	stored := 0
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// Reset inside the transaction: a retried transaction runs this closure
		// again, and a count carried over from the failed attempt would report
		// more vectors than the corpus holds.
		stored = 0
		for i, c := range batch {
			tag, err := tx.Exec(ctx,
				`UPDATE knowledge_chunk
				 SET embedding = $2::vector, embed_identity = $3, embedded_at = now()
				 WHERE id = $1 AND chunk_hash = $4 AND archived_at IS NULL
				   AND embed_identity IS NOT DISTINCT FROM $5`,
				c.id, vectorkit.Literal(vectors[i]), identity, c.hash, storedIdentityArg(c))
			if err != nil {
				return fmt.Errorf("store the passage's vector: %w", err)
			}
			// Zero rows is not an error: the passage was archived, replaced by
			// a re-ingest, or already re-stamped by a newer binding while the
			// model call was in flight, and there is nothing left for this pass
			// to say. It is not counted either — reporting a vector that was
			// not written is how a corpus reads as embedded while answering
			// nothing.
			stored += int(tag.RowsAffected())
		}
		return nil
	})
	return stored, err
}

// storedIdentityArg renders the identity a pending chunk was READ with, for the
// CAS above: SQL NULL for a row that carried none, so `IS NOT DISTINCT FROM`
// matches an unembedded row rather than failing the way `=` would against NULL.
func storedIdentityArg(c pendingChunk) *string {
	if c.storedIdentity == "" {
		return nil
	}
	return &c.storedIdentity
}
