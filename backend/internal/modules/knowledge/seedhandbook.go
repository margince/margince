// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// The operator handbook, reconciled into a corpus every time the binary starts.
//
// The product ships prose about itself, so the question "how do I reopen a
// closed deal" has an answer on the installation rather than only in a
// repository the person asking cannot see. The corpus is therefore not seeded
// once at install: it is reconciled on every boot, because the handbook belongs
// to the RELEASE and an upgrade that left the old pages in place would answer
// questions about a version that is no longer running — with a citation, which
// is a claim that the quoted text is what the handbook says.

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/knowledge/handbook"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// HandbookSource is the managed_source both the corpus and its documents carry.
// The schema's CHECK constraint holds the same set; this is the writer's half.
const HandbookSource = "handbook"

// handbookName and handbookTopic are what the corpus is called and what it says
// it is FOR.
//
// The topic statement is not decoration. It is quoted back verbatim when the
// ask refuses with not_covered, and it is the only thing on screen telling the
// reader what this set is for at the moment they are least patient — so it is
// written as an answer to "then what CAN I ask this?", not as a label.
const (
	handbookName  = "Margince handbook"
	handbookTopic = "How to operate Margince day to day: records, the pipeline, capture, " +
		"what the AI does and does not do, approvals, documents, retention and deletion, " +
		"seats and roles, and the settings pages. It does not cover this installation's " +
		"own data — no customer, deal or contact is in it."
)

// handbookContentType is fixed rather than sniffed. Every page is markdown
// authored in this repository; deriving it per file would be asking a question
// whose answer is already known and could only be wrong.
const handbookContentType = "text/markdown"

// ReconcileHandbook brings the shipped handbook corpus in line with the pages
// this binary carries, and returns the number of pages it wrote.
//
// It is idempotent and safe to run concurrently with itself: several api
// replicas boot at once during a rollout, and the caller holds an advisory lock
// for exactly that reason. The write is one transaction, so a boot that dies
// half way leaves the corpus as it was rather than half-upgraded.
//
// Called with a context already bound to the installation's workspace and a
// system actor — a boot has no request to take either from.
//
// serialize runs FIRST inside the write's own transaction. It is a parameter
// rather than a statement spelled here because the lock's key space belongs to
// the composition layer that names every other boot fact — and it has to be
// taken inside THIS transaction, since a transaction-scoped advisory lock taken
// in an earlier one is already released by the time the write it was meant to
// protect begins.
func (s *Store) ReconcileHandbook(ctx context.Context, serialize func(context.Context, pgx.Tx) error, queue QueueIngest) (int, error) {
	pages, err := handbook.Pages()
	if err != nil {
		return 0, fmt.Errorf("read the embedded handbook: %w", err)
	}
	// A build carrying no pages does not get to empty the corpus. The embed
	// pattern makes this close to unreachable, and that is the reason to check
	// rather than the reason to skip it: if it ever happens, the reconciliation
	// below would faithfully delete every page and report success, and the
	// installation would lose its handbook to a packaging mistake.
	if len(pages) == 0 {
		return 0, errors.New("knowledge: this build embeds no handbook pages; refusing to empty the shipped corpus")
	}

	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return 0, err
	}

	var written int
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := serialize(ctx, tx); err != nil {
			return err
		}
		corpusID, err := ensureHandbookCorpus(ctx, tx, by)
		if err != nil {
			return err
		}
		written, err = reconcilePages(ctx, tx, corpusID, pages, by, queue)
		return err
	})
	return written, err
}

// ensureHandbookCorpus finds the shipped corpus, creating it the first time.
//
// default_ask is set at CREATION and never re-asserted. An administrator who
// moves the palette's default to a corpus of their own has made a decision, and
// a restart is not a decision — re-claiming the flag on every boot would take
// it back at a moment nobody connected to anything they did. The corpus's NAME
// and topic statement are left alone after creation for the same reason.
func ensureHandbookCorpus(ctx context.Context, tx pgx.Tx, by string) (ids.UUID, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx,
		`SELECT id FROM knowledge_corpus WHERE managed_source = $1 AND archived_at IS NULL`,
		HandbookSource).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, fmt.Errorf("read the shipped handbook corpus: %w", err)
	}

	id = ids.NewV7()
	// The default only when nothing else holds it. A workspace that already
	// pointed the palette at a corpus of its own keeps that, and the partial
	// unique index would refuse the second default anyway — asking first turns
	// a constraint violation that fails the whole boot into an ordinary answer.
	var defaultTaken bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM knowledge_corpus WHERE default_ask AND archived_at IS NULL)`).
		Scan(&defaultTaken); err != nil {
		return ids.Nil, fmt.Errorf("read whether a default corpus exists: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO knowledge_corpus
		   (id, name, description, topic_statement, min_similarity, default_ask, managed_source, captured_by)
		 VALUES ($1, $2, NULL, $3, $4, $5, $6, $7)`,
		id, handbookName, handbookTopic, DefaultMinSimilarity, !defaultTaken, HandbookSource, by); err != nil {
		return ids.Nil, fmt.Errorf("create the shipped handbook corpus: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "create", "knowledge_corpus", id, nil, map[string]any{
		"name":            handbookName,
		"topic_statement": handbookTopic,
		"min_similarity":  DefaultMinSimilarity,
		defaultAskColumn:  !defaultTaken,
		managedSourceKey:  HandbookSource,
	}); err != nil {
		return ids.Nil, fmt.Errorf("audit the shipped handbook corpus: %w", err)
	}
	return id, nil
}

// reconcilePages makes the corpus's managed documents match the pages this
// build ships: added, updated by checksum, and removed when withdrawn.
func reconcilePages(ctx context.Context, tx pgx.Tx, corpusID ids.UUID, pages []handbook.Page, by string, queue QueueIngest) (int, error) {
	existing, err := managedDocuments(ctx, tx, corpusID)
	if err != nil {
		return 0, err
	}

	var written int
	for _, page := range pages {
		checksum, size, err := blobstore.Digest(bytes.NewReader(page.Content))
		if err != nil {
			return 0, err
		}
		prior, known := existing[page.Filename]
		delete(existing, page.Filename)
		// Unchanged pages are skipped WITHOUT re-queueing an ingest. Every boot
		// runs this, and a rollout restarts every replica: re-ingesting eleven
		// pages each time would spend an embedding call per page per boot to
		// arrive at the passages already in the table.
		if known && prior.checksum == checksum {
			continue
		}
		if known {
			err = replacePage(ctx, tx, corpusID, page, checksum, size, prior, queue)
		} else {
			err = filePage(ctx, tx, corpusID, page, checksum, size, by, queue)
		}
		if err != nil {
			return 0, err
		}
		written++
	}

	// Whatever is left was shipped by an earlier release and is not in this
	// one. Its passages go with it — a citation into a withdrawn page quotes
	// prose the product no longer stands behind.
	for filename, doc := range existing {
		if err := deleteChunksOf(ctx, tx, doc.id); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM knowledge_document WHERE id = $1`, doc.id); err != nil {
			return 0, fmt.Errorf("remove the withdrawn handbook page: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "delete", "knowledge_document", doc.id,
			map[string]any{filenameKey: filename, managedSourceKey: HandbookSource}, nil); err != nil {
			return 0, fmt.Errorf("audit the withdrawn handbook page: %w", err)
		}
		written++
	}
	return written, nil
}

// managedDoc is what the reconciliation needs to know about a page already
// filed: which row it is, and whether its bytes are still current.
type managedDoc struct {
	id       ids.UUID
	checksum string
}

func managedDocuments(ctx context.Context, tx pgx.Tx, corpusID ids.UUID) (map[string]managedDoc, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, filename, checksum FROM knowledge_document
		 WHERE corpus_id = $1 AND managed_source = $2 AND archived_at IS NULL`,
		corpusID, HandbookSource)
	if err != nil {
		return nil, fmt.Errorf("read the filed handbook pages: %w", err)
	}
	defer rows.Close()
	out := make(map[string]managedDoc)
	for rows.Next() {
		var doc managedDoc
		var filename string
		if err := rows.Scan(&doc.id, &filename, &doc.checksum); err != nil {
			return nil, fmt.Errorf("read a filed handbook page: %w", err)
		}
		out[filename] = doc
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the filed handbook pages: %w", err)
	}
	return out, nil
}

// filePage records a page this installation has not carried before.
func filePage(ctx context.Context, tx pgx.Tx, corpusID ids.UUID, page handbook.Page, checksum string, size int64, by string, queue QueueIngest) error {
	id := ids.NewV7()
	if _, err := tx.Exec(ctx,
		`INSERT INTO knowledge_document
		   (id, corpus_id, filename, content_type, byte_size, storage_key, checksum, managed_source, captured_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, corpusID, page.Filename, handbookContentType, size,
		handbookStorageKey(page.Filename), checksum, HandbookSource, by); err != nil {
		return fmt.Errorf("file the handbook page: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "create", "knowledge_document", id, nil,
		pageImage(corpusID, page.Filename, size)); err != nil {
		return fmt.Errorf("audit the filed handbook page: %w", err)
	}
	return queueIngestOf(ctx, tx, id, queue)
}

// replacePage points an existing page's row at this release's bytes.
//
// The ROW and its id survive. The id is what a citation and a download URL are
// built from, so replacing the row would break every link in an answer a reader
// still has open, in order to say that a paragraph was reworded.
func replacePage(ctx context.Context, tx pgx.Tx, corpusID ids.UUID, page handbook.Page, checksum string, size int64, prior managedDoc, queue QueueIngest) error {
	// The passages came from the OLD bytes. Clearing them HERE rather than
	// leaving it to the ingest means a reader between this commit and the
	// ingest finishing gets not_ready — honest — instead of an answer grounded
	// in prose this release replaced.
	if err := deleteChunksOf(ctx, tx, prior.id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE knowledge_document
		    SET byte_size = $2, checksum = $3, chunk_count = 0,
		        ingest_status = 'queued', ingest_started_at = NULL, ingest_detail = NULL
		  WHERE id = $1`,
		prior.id, size, checksum); err != nil {
		return fmt.Errorf("update the handbook page: %w", err)
	}
	if _, err := storekit.Audit(ctx, tx, "update", "knowledge_document", prior.id,
		map[string]any{"checksum": prior.checksum},
		pageImage(corpusID, page.Filename, size)); err != nil {
		return fmt.Errorf("audit the replaced handbook page: %w", err)
	}
	return queueIngestOf(ctx, tx, prior.id, queue)
}

// pageImage is the audit image both writes record. One spelling, because two
// audit images of one entity that disagree on their keys make a document's
// history unreadable as a sequence.
//
// It carries the page's NAME and SIZE and never its prose: audit_log is
// append-only and outlives the delete that clears what it quotes, which is the
// same reason the document module pins its own images to four keys.
func pageImage(corpusID ids.UUID, filename string, size int64) map[string]any {
	return map[string]any{
		corpusIDKey:      corpusID,
		filenameKey:      filename,
		contentTypeKey:   handbookContentType,
		byteSizeKey:      size,
		managedSourceKey: HandbookSource,
	}
}

// queueIngestOf enqueues a page's ingest INSIDE the row's own transaction, for
// the reason the upload path states: a job enqueued outside it can wake before
// the row exists, or survive a rollback that leaves it pointing at nothing.
func queueIngestOf(ctx context.Context, tx pgx.Tx, id ids.UUID, queue QueueIngest) error {
	if err := queue(ctx, tx, id); err != nil {
		return fmt.Errorf("queue the handbook page's ingest: %w", err)
	}
	return nil
}
