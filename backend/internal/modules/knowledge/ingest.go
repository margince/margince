// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// Turning a stored document into the chunks an ask retrieves over.
//
// The lifecycle here is written so a RETRY is indistinguishable from a first
// attempt. River retries an ingest up to its declared attempts, and an attempt
// that crashed halfway has already written chunks; the rule that makes that
// safe is stated once and enforced in one place:
//
//	A chunk exists only for a document whose ingest is queued, running or done.
//	An attempt BEGINS by deleting whatever the previous attempt wrote for that
//	document, and a terminally failed ingest deletes them too.
//
// So an attempt is idempotent on restart, a retry needs no un-stamping, and a
// failed document has no chunks left to leak into an answer.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// maxCorpusChunks is the ceiling on ONE corpus, not one file.
//
// A per-file cap is the wrong bound: N small uploads exceed any of them, and
// the cost this protects is the ask's, which scans the corpus. The ask has no
// vector index to lean on — the column is unbounded width, so an index over it
// is unusable — which makes every chunk in the corpus work the question does.
const maxCorpusChunks = 20000

// CorpusFullError is a document refused because the corpus it would join has no
// room. It names the ceiling, because "too large" without a number leaves the
// uploader guessing at which file to remove.
type CorpusFullError struct {
	Have, Adding, Ceiling int
}

func (e *CorpusFullError) Error() string {
	return fmt.Sprintf("this corpus holds %d passages and this document adds %d, past the %d-passage ceiling one corpus may reach",
		e.Have, e.Adding, e.Ceiling)
}

// IngestSource is the stored document an attempt reads: where its bytes are,
// and which corpus its chunks join.
type IngestSource struct {
	CorpusID   ids.UUID
	StorageKey string
}

// BeginIngest opens an attempt: the document moves to running, and whatever a
// previous attempt wrote for it is deleted.
//
// The delete is the whole retry protocol. Without it a crashed attempt's chunks
// are counted by the next one's ceiling check and cited twice by every answer.
func (s *Store) BeginIngest(ctx context.Context, documentID ids.UUID) (IngestSource, error) {
	var src IngestSource
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx,
			`SELECT corpus_id, storage_key FROM knowledge_document
			 WHERE id = $1 AND archived_at IS NULL`, documentID).
			Scan(&src.CorpusID, &src.StorageKey); err != nil {
			return notFoundOr(err, "read the document to ingest")
		}
		if err := deleteChunksOf(ctx, tx, documentID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx,
			`UPDATE knowledge_document
			 SET ingest_status = 'running', ingest_detail = NULL, chunk_count = 0
			 WHERE id = $1 AND archived_at IS NULL`, documentID)
		if err != nil {
			return fmt.Errorf("open the document's ingest: %w", err)
		}
		return nil
	})
	return src, err
}

// WriteChunks records this attempt's spans, refusing the write if the corpus
// would pass its ceiling.
//
// ONE audit row on the DOCUMENT, never one per chunk: the chunks are a derived
// artifact of one act, and a thousand audit rows for one upload is a trail
// nobody can read.
//
// The document's archived_at is re-read INSIDE this transaction so an archive
// that raced the attempt wins: the archive stamps the chunks that exist when it
// runs, and without this re-read an attempt already past that point would write
// live chunks under an archived document a moment later.
func (s *Store) WriteChunks(ctx context.Context, documentID ids.UUID, corpusID ids.UUID, chunks []Chunk) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		var archived *string
		var wasCount int
		if err := tx.QueryRow(ctx,
			`SELECT archived_at::text, chunk_count FROM knowledge_document WHERE id = $1 FOR UPDATE`, documentID).
			Scan(&archived, &wasCount); err != nil {
			return notFoundOr(err, "re-read the document before writing its chunks")
		}
		if archived != nil {
			return apperrors.ErrNotFound
		}
		var have int
		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM knowledge_chunk WHERE corpus_id = $1 AND archived_at IS NULL`,
			corpusID).Scan(&have); err != nil {
			return fmt.Errorf("count the corpus's passages: %w", err)
		}
		if have+len(chunks) > maxCorpusChunks {
			return &CorpusFullError{Have: have, Adding: len(chunks), Ceiling: maxCorpusChunks}
		}
		if err := insertChunks(ctx, tx, documentID, corpusID, chunks); err != nil {
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "update", "knowledge_document", documentID,
			map[string]any{"chunk_count": wasCount},
			map[string]any{"chunk_count": len(chunks)}); err != nil {
			return fmt.Errorf("audit the document's chunking: %w", err)
		}
		return nil
	})
}

// insertChunks writes the spans in one statement. The placeholders are DERIVED
// from the argument slice rather than typed: a hand-written $N list is one edit
// away from a statement whose columns, placeholders and arguments disagree, and
// nothing in the build checks that they agree.
func insertChunks(ctx context.Context, tx pgx.Tx, documentID, corpusID ids.UUID, chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	args := make([]any, 0, len(chunks)*6)
	rows := make([]string, 0, len(chunks))
	for _, c := range chunks {
		rows = append(rows, storekit.SQLf("($%d, $%d, $%d, $%d, $%d, $%d)",
			len(args)+1, len(args)+2, len(args)+3, len(args)+4, len(args)+5, len(args)+6))
		args = append(args, ids.NewV7(), corpusID, documentID, c.Ix, c.Text, c.Hash())
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO knowledge_chunk (id, corpus_id, document_id, chunk_ix, text, chunk_hash)
		 VALUES `+strings.Join(rows, ", "), args...); err != nil {
		return fmt.Errorf("write the document's passages: %w", err)
	}
	return nil
}

// FinishIngest closes a successful attempt.
func (s *Store) FinishIngest(ctx context.Context, documentID ids.UUID, chunkCount int) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE knowledge_document SET ingest_status = 'done', ingest_detail = NULL, chunk_count = $2
			 WHERE id = $1 AND archived_at IS NULL`, documentID, chunkCount)
		if err != nil {
			return fmt.Errorf("close the document's ingest: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return apperrors.ErrNotFound
		}
		return nil
	})
}

// FailIngest closes an attempt terminally, with a reason the uploader can act
// on, and takes this document's chunks with it.
//
// It is called only once River's attempts are exhausted. A document mid-retry
// stays `running`, which matters because readiness reads that word: a corpus
// with an attempt still to come is not ready, and is not permanently broken
// either.
func (s *Store) FailIngest(ctx context.Context, documentID ids.UUID, detail string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		var wasStatus string
		var wasDetail *string
		if err := tx.QueryRow(ctx,
			`SELECT ingest_status, ingest_detail FROM knowledge_document
			 WHERE id = $1 AND archived_at IS NULL FOR UPDATE`, documentID).
			Scan(&wasStatus, &wasDetail); err != nil {
			return notFoundOr(err, "re-read the document before failing its ingest")
		}
		if err := deleteChunksOf(ctx, tx, documentID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`UPDATE knowledge_document SET ingest_status = 'failed', ingest_detail = $2, chunk_count = 0
			 WHERE id = $1 AND archived_at IS NULL`, documentID, detail)
		if err != nil {
			return fmt.Errorf("fail the document's ingest: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return apperrors.ErrNotFound
		}
		if _, err := storekit.Audit(ctx, tx, "update", "knowledge_document", documentID,
			map[string]any{"ingest_status": wasStatus, "ingest_detail": wasDetail},
			map[string]any{"ingest_status": "failed", "ingest_detail": detail}); err != nil {
			return fmt.Errorf("audit the failed ingest: %w", err)
		}
		return nil
	})
}

// OpenDocument streams a stored document's bytes.
func (s *Store) OpenDocument(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	if s.blob == nil {
		return nil, ErrBlobstoreUnconfigured
	}
	body, _, err := s.blob.Get(ctx, storageKey)
	if err != nil {
		return nil, fmt.Errorf("read the stored document: %w", err)
	}
	return body, nil
}

func deleteChunksOf(ctx context.Context, tx pgx.Tx, documentID ids.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM knowledge_chunk WHERE document_id = $1`, documentID); err != nil {
		return fmt.Errorf("clear the previous attempt's passages: %w", err)
	}
	return nil
}

func notFoundOr(err error, op string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}
