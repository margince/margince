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
// Without this sweep that state is permanent. Nothing else re-embeds an
// existing document: an ingest runs once per upload, and a person who
// re-uploaded the same file to "fix" it would get a SECOND document rather than
// a repaired one — every passage twice, competing with itself for the eight
// retrieval slots. So the repair is not something a person should have to know
// to ask for, and it is not something they could ask for correctly if they did.

import (
	"context"
	"errors"
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
	// Lowered FIRST, on every corpus with nothing stale left. The flag is
	// raised per document and lowered on the way out, but a pass that DIED
	// between the last document's vectors committing and its own flag-lowering
	// leaves it raised — and the next pass then finds nothing stale, so nothing
	// would ever lower it. The corpus would report not_ready forever while
	// holding a complete, current index.
	//
	// So the flag is not something a sweep must remember to clean up: it is
	// derived here from what the corpus actually holds, which makes a crashed
	// pass self-healing rather than a wedge.
	if err := s.clearSettledReindexing(ctx, identity); err != nil {
		return 0, err
	}
	documents, err := s.documentsUnderAStaleBinding(ctx, identity)
	if err != nil {
		return 0, err
	}
	repaired := 0
	var failures []error
	for _, d := range documents {
		n, err := s.reembedDocument(ctx, d, e)
		repaired += n
		if err != nil {
			// The pass CONTINUES past a document it could not repair, and this
			// is the difference between a sweep and a wedge. Returning here
			// stopped at the first failure, and because the sweep is periodic
			// and the document order is stable, the same document stopped it
			// every time — so ONE passage a provider will not embed left every
			// later document in the workspace under a superseded binding
			// forever, unaskable, with nothing in the corpus to explain it.
			//
			// The failures are joined and returned, so River still sees a fault
			// and the operator still sees the reason; what changes is that the
			// documents behind the bad one are repaired first.
			failures = append(failures, fmt.Errorf("re-embedding document %s: %w", d.id, err))
		}
	}
	// The count is returned whatever happened: the passages already repaired
	// ARE repaired, and reporting zero would make a partial pass
	// indistinguishable from one that did nothing.
	return repaired, errors.Join(failures...)
}

// staleDocument is one document needing re-embedding, and the corpus it is in.
type staleDocument struct {
	id     ids.UUID
	corpus ids.UUID
}

// documentsUnderAStaleBinding finds the documents holding at least one passage
// whose vector is not the live binding's, oldest first.
//
// ORDERED, because a sweep whose order is whatever the planner chose is a sweep
// whose behaviour on a document it cannot repair cannot be reasoned about or
// tested. Oldest first is also the order a person would expect: the document
// that has been wrong longest is repaired first.
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
			    AND c.embed_identity IS DISTINCT FROM $1
			  ORDER BY c.document_id`, identity)
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

// clearSettledReindexing lowers the flag on every corpus that has no passage
// left under a superseded binding.
//
// Written as ONE statement over the whole workspace rather than per corpus,
// because the question it answers is per corpus and the answer is derivable:
// a corpus marked reindexing with nothing stale in it is finished, whatever
// the pass that marked it went on to do.
func (s *Store) clearSettledReindexing(ctx context.Context, identity string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE knowledge_corpus k SET reindexing = false
			  WHERE k.reindexing
			    AND k.archived_at IS NULL
			    AND NOT EXISTS (
			      SELECT 1 FROM knowledge_chunk c
			       JOIN knowledge_document d ON d.id = c.document_id
			      WHERE c.corpus_id = k.id
			        AND c.archived_at IS NULL
			        AND d.archived_at IS NULL
			        AND d.ingest_status = 'done'
			        AND c.embed_identity IS DISTINCT FROM $1)`, identity); err != nil {
			return fmt.Errorf("lower the reindexing flag on the settled corpora: %w", err)
		}
		return nil
	})
}

// abandonedAfter is how long a document may sit `running` before the sweep
// gives up on it.
//
// It is the ingest job's own wall clock (15m, api/jobs.yaml) plus a wide
// margin, because the two failures it must tell apart are "slow" and "gone". A
// job still inside its timeout is slow and must be left alone; one whose
// timeout has passed with the row untouched has no worker behind it — River
// rescues a job whose worker died, but a process that dies on the LAST attempt
// leaves nothing to rescue and no attempt to run.
//
// Without this the document sits `running` forever, readiness reads it as
// in-flight, and the WHOLE CORPUS answers not_ready permanently — every other
// document in it unaskable because one upload's worker was killed. That is a
// corpus bricked by a machine restart.
const abandonedAfter = "1 hour"

// SweepAbandonedIngests fails the documents whose ingest stopped without
// saying so, and reports how many it closed.
//
// It rides the drift sweep for the same reason the corpus repair does: the
// sweep is already periodic and per-workspace, and a second one would be a
// second answer to "is this corpus in a state it can be asked in".
//
// The threshold is compared in SQL against ingest_started_at, so the DATABASE's
// clock decides and the column says what it needs to know. updated_at would be
// wrong: the trigger moves it for any write, so a row touched for any other
// reason would look like a fresh attempt and never be swept.
//
// There is no clock to inject and none to get wrong, and a test backdates the
// column rather than waiting.
func (s *Store) SweepAbandonedIngests(ctx context.Context) (int, error) {
	var abandoned []ids.UUID
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id FROM knowledge_document
			  WHERE ingest_status = 'running'
			    AND archived_at IS NULL
			    AND ingest_started_at < now() - $1::interval`, abandonedAfter)
		if err != nil {
			return fmt.Errorf("find the abandoned ingests: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			abandoned = append(abandoned, id)
		}
		return rows.Err()
	}); err != nil {
		return 0, err
	}
	closed := 0
	for _, id := range abandoned {
		// Through FailIngest, not a bare UPDATE: the passages a half-finished
		// attempt wrote have to go, the stored file has to go, and the audit
		// row is owed. Spelling any of that again here would be a second
		// writer of the same terminal state.
		if err := s.FailIngest(ctx, id,
			"Reading this document stopped without finishing — the machine doing it went away. Delete it and upload it again."); err != nil {
			return closed, err
		}
		closed++
	}
	return closed, nil
}
