// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// Repairing a corpus after the embed binding moves.
//
// An operator swapping the embed lane leaves every stored vector in a space the
// live query no longer shares. The ask's identity filter then excludes all of
// them — correctly, because ranking across two spaces is meaningless and
// comparing two widths raises an error outright — so the corpus retrieves
// nothing and reports `not_ready`.
//
// Without this sweep that state is permanent. Re-uploading the same file does
// not fix it: the checksum matches, so nothing re-chunks, and there is no other
// path that re-embeds. That is a corpus bricked by a configuration change, and
// the repair is not something a person should have to know to ask for.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/vectorkit"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// SweepCorpusDrift re-embeds every passage whose vector was computed under a
// binding that is no longer live, and reports how many it repaired.
//
// It rides the existing embed drift sweep rather than a job of its own. That
// sweep already answers "has the live identity moved", and a second one would
// be a second answer to one question — the two would disagree the first time
// either changed. It is periodic and per-workspace, its timeout is deliberately
// absent because it is bounded by a backlog rather than a clock, and its
// consent posture is exactly this one's: healing drift is the same spend class
// as the ingest that missed it, so no human confirms it.
//
// Resumable for free: a crashed pass re-runs and skips whatever the previous
// one finished, because vectorkit.Unchanged is what decides each passage.
func (s *Store) SweepCorpusDrift(ctx context.Context, e vectorkit.Embedder) (int, error) {
	identity, _ := e.EmbedIdentity()
	if identity == "" {
		// No embed lane bound. Nothing has drifted, because nothing is live to
		// have drifted from — and re-embedding into no space at all is not a
		// repair.
		return 0, nil
	}
	documents, err := s.documentsUnderAStaleBinding(ctx, identity)
	if err != nil {
		return 0, err
	}
	repaired := 0
	for _, d := range documents {
		n, err := s.reembedDocument(ctx, d, e)
		repaired += n
		if err != nil {
			// The count so far is returned with the error: the passages already
			// repaired ARE repaired, and reporting zero would make a partial
			// pass indistinguishable from one that did nothing.
			return repaired, err
		}
	}
	return repaired, nil
}

// staleDocument is one document needing re-embedding, and the corpus it is in.
type staleDocument struct {
	id     ids.UUID
	corpus ids.UUID
}

// documentsUnderAStaleBinding finds the documents holding at least one passage
// whose vector is not the live binding's.
//
// A NULL identity counts as stale: those are passages an ingest wrote but never
// embedded — an installation that had no lane when the document landed and has
// one now. Repairing them here is the same act as repairing a swap, and leaving
// them out would mean a corpus uploaded before the lane was bound stayed
// permanently unanswerable in a different way.
func (s *Store) documentsUnderAStaleBinding(ctx context.Context, identity string) ([]staleDocument, error) {
	var out []staleDocument
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT DISTINCT c.document_id, c.corpus_id
			   FROM knowledge_chunk c
			   JOIN knowledge_document d ON d.id = c.document_id
			   JOIN knowledge_corpus k ON k.id = c.corpus_id
			  WHERE c.archived_at IS NULL
			    AND d.archived_at IS NULL
			    AND k.archived_at IS NULL
			    AND d.ingest_status = 'done'
			    AND c.embed_identity IS DISTINCT FROM $1`, identity)
		if err != nil {
			return fmt.Errorf("find the passages under a stale binding: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var d staleDocument
			if err := rows.Scan(&d.id, &d.corpus); err != nil {
				return err
			}
			out = append(out, d)
		}
		return rows.Err()
	})
	return out, err
}

// reembedDocument repairs one document under the reindexing flag.
//
// The flag is what lets the corpus screen and the ask say "re-embedding" rather
// than "not ready". They are different statements: not_ready invites the reader
// to go and finish uploading something, and a corpus mid-repair has nothing for
// them to do but wait.
//
// It is cleared on the way out whatever happened, including on failure. A flag
// left raised by a crashed pass would make a corpus that has simply stopped
// being repaired look like one that is still being repaired, and nothing would
// ever lower it.
func (s *Store) reembedDocument(ctx context.Context, d staleDocument, e vectorkit.Embedder) (int, error) {
	if err := s.setReindexing(ctx, d.corpus, true); err != nil {
		return 0, err
	}
	repaired, embedErr := s.EmbedDocument(ctx, d.id, e)
	if err := s.setReindexing(ctx, d.corpus, false); err != nil {
		// The embed's own failure is the one worth reporting; a flag that could
		// not be lowered is reported only when nothing else went wrong.
		if embedErr != nil {
			return repaired, embedErr
		}
		return repaired, err
	}
	return repaired, embedErr
}

func (s *Store) setReindexing(ctx context.Context, corpusID ids.UUID, on bool) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE knowledge_corpus SET reindexing = $2 WHERE id = $1 AND archived_at IS NULL`,
			corpusID, on); err != nil {
			return fmt.Errorf("mark the corpus's reindexing state: %w", err)
		}
		return nil
	})
}
