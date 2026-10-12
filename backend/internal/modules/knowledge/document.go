// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// A document arriving in a corpus: the bytes go to object storage, the row
// records where, and the ingest that turns it into chunks is queued behind it.

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// NewDocument is one uploaded file. Content is a reader rather than bytes: the
// store hashes it and streams it to object storage, so a file never has to be
// resident whole.
type NewDocument struct {
	CorpusID    ids.UUID
	Filename    string
	ContentType string
	Content     io.ReadSeeker
}

// QueueIngest is how the composition layer enqueues the ingest job inside the
// transaction that writes the document row. The module declares the seam and
// never learns what a job is: an ingest queued outside the row's transaction
// can run against a row that never commits, or be lost when one does.
type QueueIngest func(ctx context.Context, tx pgx.Tx, documentID ids.UUID) error

// UploadDocument stores the file and records it queued for ingest.
//
// The corpus is checked BEFORE any bytes are written, so an upload to a corpus
// that does not exist cannot land an object. The object is put before the row
// commits: a committed row always has its bytes, and a failed write leaves at
// worst an orphan object rather than a row promising bytes that are not there.
func (s *Store) UploadDocument(ctx context.Context, in NewDocument, queue QueueIngest) (crmcontracts.KnowledgeDocument, error) {
	media, checksum, size, err := s.readyUpload(ctx, &in)
	if err != nil {
		return crmcontracts.KnowledgeDocument{}, err
	}

	id := ids.NewV7()
	key := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](storekit.MustWorkspace(ctx)),
		string(storedobjects.KindKnowledge), id.String())
	// Declared BEFORE the put, on its own transaction, so the declaration survives
	// the failure of the one below. The cleanup after this transaction covers the
	// ordinary refusal; what it cannot cover is this process dying between here and
	// there, or the delete itself being refused.
	if err := storedobjects.NewLedger(s.db).Record(ctx, storedobjects.KindKnowledge, key); err != nil {
		return crmcontracts.KnowledgeDocument{}, err
	}
	if err := s.blob.Put(ctx, key, in.Content, size, media); err != nil {
		return crmcontracts.KnowledgeDocument{}, fmt.Errorf("store the corpus document: %w", err)
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.KnowledgeDocument{}, err
	}

	var out crmcontracts.KnowledgeDocument
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// Re-read under the write's own transaction: a corpus archived while
		// the bytes were being stored would otherwise take a live document.
		if _, err := readCorpus(ctx, tx, in.CorpusID, storekit.LiveOnly); err != nil {
			return err
		}
		// Checked AGAIN here, and this is the one that decides: the pre-flight
		// spares the storage write in the ordinary case, and only a check
		// inside this transaction is safe against a second upload of the same
		// bytes racing it.
		existing, err := documentHoldingIn(ctx, tx, in.CorpusID, checksum)
		if err != nil {
			return err
		}
		if existing.found && !existing.failed {
			return &AlreadyFiledError{Filename: existing.filename}
		}
		if existing.failed {
			// Discarded, so the insert below files these bytes as a new
			// document. That is the manual step the advice already requires:
			// a re-upload only worked once somebody deleted the failed row.
			if err := discardFailedDocument(ctx, tx, existing.id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO knowledge_document
			   (id, corpus_id, filename, content_type, byte_size, storage_key, checksum, captured_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			id, in.CorpusID, in.Filename, media, size, key, checksum, by); err != nil {
			// The index catching what the read above could not: two uploads of
			// identical bytes can both pass that read before either commits.
			// The loser gets the SAME refusal the reader would have given,
			// rather than a constraint name.
			//
			// Without the winner's NAME, and that is not a shortcut: this
			// transaction is already aborted by the violation, so a query for
			// it here would fail. The name is what the ordinary path supplies;
			// this path is the rare one, and saying less is better than
			// failing differently.
			if storekit.IsUniqueViolation(err) {
				return &AlreadyFiledError{}
			}
			return fmt.Errorf("insert corpus document: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "create", "knowledge_document", id, nil, map[string]any{
			corpusIDKey:    in.CorpusID,
			filenameKey:    in.Filename,
			contentTypeKey: media,
			byteSizeKey:    size,
		}); err != nil {
			return fmt.Errorf("audit corpus document create: %w", err)
		}
		// Queued INSIDE the row's transaction: a job enqueued outside it can
		// wake before the row exists, or survive a rollback that leaves it
		// pointing at nothing.
		if err := queue(ctx, tx, id); err != nil {
			return fmt.Errorf("queue the corpus document's ingest: %w", err)
		}
		// Retired on THIS transaction, so the clear and the row commit together: a
		// clear that committed separately could land while this then failed, which
		// is the orphan the ledger exists to catch, one step along.
		if err := storedobjects.Clear(ctx, tx, key); err != nil {
			return err
		}
		var rerr error
		out, rerr = readDocument(ctx, tx, id)
		return rerr
	})
	if err != nil {
		// The bytes were written before this transaction opened, and no row
		// now names them. The ordinary case is the duplicate race: two uploads
		// of identical bytes both clear the pre-flight, both call Put, and the
		// loser is refused by the unique index — leaving its own object behind
		// with nothing pointing at it. The ledger would collect it a grace
		// period later, since every sweep in this module walks ROWS and only
		// that one walks keys; deleting it here is what makes the ordinary
		// refusal cost nothing rather than wait a day.
		//
		// Reported rather than swallowed, and joined rather than substituted:
		// the caller's refusal is the answer they need, and a storage backend
		// that would not delete is a separate fault an operator needs to see.
		if derr := s.blob.Delete(ctx, key); derr != nil {
			// The intent STAYS when the delete fails, which is the ledger's whole
			// contract: a key is retired only once its bytes are gone, so the sweep
			// finishes what this could not.
			return crmcontracts.KnowledgeDocument{}, errors.Join(err,
				fmt.Errorf("delete the corpus document's stored object after a failed write: %w", derr))
		}
		// The bytes are gone, so the declaration has nothing left to describe: left
		// standing it would occupy a slot in every sweep's limit for a day, to be
		// adjudicated as an orphan whose object is already deleted.
		//
		// Cleared on a transaction of this store's own, not Retire: retiring is the
		// SWEEP's act and takes the system principal, and this path runs as the
		// contact who uploaded the file. Reaching for it here answered their server
		// fault with a 403.
		if rerr := s.tx(ctx, func(tx pgx.Tx) error {
			return storedobjects.Clear(ctx, tx, key)
		}); rerr != nil {
			return crmcontracts.KnowledgeDocument{}, errors.Join(err, rerr)
		}
		return crmcontracts.KnowledgeDocument{}, err
	}
	return out, nil
}

// readyUpload settles everything an upload can be refused for BEFORE any bytes
// are stored: the grant, the object store, the file's type, the corpus, and
// whether these exact bytes are already filed.
//
// Split out because each of those is a separate reason and the list only grows
// — and because a refusal that happens after blob.Put has already written the
// object leaves bytes no row names.
//
// It normalises in.Filename in place, so the name that reaches the column is
// the one that was hashed and refused against.
func (s *Store) readyUpload(ctx context.Context, in *NewDocument) (string, string, int64, error) {
	if err := auth.Require(ctx, "knowledge_document", principal.ActionCreate); err != nil {
		return "", "", 0, err
	}
	if s.blob == nil {
		return "", "", 0, ErrBlobstoreUnconfigured
	}
	// The name a contact TYPED, made safe — the same call every other uploaded
	// name in this tree goes through. It is presentational only (nothing opens
	// a file by it) but it is read back in a citation, a list and an ingest
	// failure, and a path separator or a bidirectional override in it rewrites
	// whichever of those quotes it.
	//
	// Normalised HERE, above the type check, rather than after it: the 415
	// detail quotes this name when no media type could be derived, so a
	// refusal is one of the surfaces that reads it back. Sanitising afterwards
	// left the one path that handles hostile input quoting it raw.
	in.Filename = extension.SafeFilename(in.Filename, 0)

	media, ok := acceptedType(in.ContentType, in.Filename)
	if !ok {
		return "", "", 0, &UnsupportedTypeError{Got: refusedTypeName(media, *in)}
	}
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		_, err := readCorpus(ctx, tx, in.CorpusID, storekit.LiveOnly)
		return err
	}); err != nil {
		return "", "", 0, err
	}
	checksum, size, err := blobstore.Digest(in.Content)
	if err != nil {
		return "", "", 0, err
	}
	// Refused BEFORE the bytes are stored. Checked again inside the write —
	// that is the one that decides — but a re-upload is the ORDINARY case here,
	// and probing only there wrote the whole object to storage and then rolled
	// the row back, leaving bytes no row names.
	existing, err := s.documentHolding(ctx, in.CorpusID, checksum)
	if err != nil {
		return "", "", 0, err
	}
	// A failed row holds no bytes, so it is not a reason to refuse.
	//
	// The write discards it and files these bytes fresh.
	if existing.found && !existing.failed {
		return "", "", 0, &AlreadyFiledError{Filename: existing.filename}
	}
	return media, checksum, size, nil
}

// holdingRow is the live document already carrying these bytes, if one is.
//
// `failed` is carried because such a row holds no bytes: FailIngest deletes the
// stored object and keeps the row.
//
// The advice on screen is to upload the file again, and a re-upload refused as
// already filed made that impossible to follow.
type holdingRow struct {
	id       ids.UUID
	filename string
	failed   bool
	found    bool
}

func documentHoldingIn(ctx context.Context, tx pgx.Tx, corpusID ids.UUID, checksum string) (holdingRow, error) {
	var row holdingRow
	switch err := tx.QueryRow(ctx,
		`SELECT id, filename, ingest_status = 'failed' FROM knowledge_document
		  WHERE corpus_id = $1 AND checksum = $2 AND archived_at IS NULL
		  LIMIT 1`, corpusID, checksum).Scan(&row.id, &row.filename, &row.failed); {
	case err == nil:
		row.found = true
		return row, nil
	case errors.Is(err, pgx.ErrNoRows):
		return holdingRow{}, nil
	default:
		return holdingRow{}, fmt.Errorf("look for a document already holding these bytes: %w", err)
	}
}

// documentHolding names the live document in this corpus already holding these
// exact bytes, or "" when none does.
func (s *Store) documentHolding(ctx context.Context, corpusID ids.UUID, checksum string) (holdingRow, error) {
	var existing holdingRow
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		existing, err = documentHoldingIn(ctx, tx, corpusID, checksum)
		return err
	})
	return existing, err
}

// ListDocuments returns one corpus's live documents, newest first, whatever
// state each one's ingest reached.
//
// A failed document is listed with its reason. The coverage counts say how much
// of a corpus is searchable; this says which files those counts are made of,
// and a corpus quietly short of a file nobody can name answers worse than an
// empty one, because it still answers.
func (s *Store) ListDocuments(ctx context.Context, corpusID ids.UUID) ([]crmcontracts.KnowledgeDocument, error) {
	if err := auth.Require(ctx, "knowledge_document", principal.ActionRead); err != nil {
		return nil, err
	}
	out := []crmcontracts.KnowledgeDocument{}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		// The corpus read first, so an unknown or archived corpus is absent
		// rather than an empty list — "no documents" and "no such corpus" are
		// different answers and the screen renders them differently.
		if _, err := readCorpus(ctx, tx, corpusID, storekit.LiveOnly); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT `+documentColumns+` FROM knowledge_document
			 WHERE corpus_id = $1 AND archived_at IS NULL
			 ORDER BY created_at DESC, id DESC`, corpusID)
		if err != nil {
			return fmt.Errorf("list corpus documents: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			doc, err := scanDocument(rows)
			if err != nil {
				return err
			}
			out = append(out, doc)
		}
		return rows.Err()
	})
	return out, err
}

const documentColumns = `id, corpus_id, filename, content_type, byte_size,
	ingest_status, ingest_detail, chunk_count, created_at`

func readDocument(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.KnowledgeDocument, error) {
	doc, err := scanDocument(tx.QueryRow(ctx,
		`SELECT `+documentColumns+` FROM knowledge_document WHERE id = $1 AND archived_at IS NULL`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.KnowledgeDocument{}, apperrors.ErrNotFound
	}
	return doc, err
}

func scanDocument(row pgx.Row) (crmcontracts.KnowledgeDocument, error) {
	var doc crmcontracts.KnowledgeDocument
	var id, corpusID ids.UUID
	var status string
	var chunkCount int
	if err := row.Scan(&id, &corpusID, &doc.Filename, &doc.ContentType, &doc.ByteSize,
		&status, &doc.IngestDetail, &chunkCount, &doc.CreatedAt); err != nil {
		return crmcontracts.KnowledgeDocument{}, err
	}
	doc.Id = openapi_types.UUID(id)
	doc.CorpusId = openapi_types.UUID(corpusID)
	doc.IngestStatus = crmcontracts.KnowledgeDocumentIngestStatus(status)
	doc.ChunkCount = &chunkCount
	return doc, nil
}

// DeleteDocument removes a document, its passages, their vectors and its
// stored file.
//
// A HARD delete, not an archive. An embedding is the document's text in another
// shape — a similarity probe reconstructs neighbourhoods of what it was made
// from — and a stored original left behind would make the deletion decorative.
//
// It REFUSES when no object store is bound rather than deleting the rows and
// leaving the bytes. A partial delete that reports success is the worst
// available outcome: the screen says the document is gone, and the file it was
// made from is still there with nothing left pointing at it to find it by.
//
// The blob goes LAST, after the rows commit, and the ordering has a cost worth
// stating rather than implying away.
//
// The other order deletes bytes that a failed transaction then leaves a live
// row promising — a document the screen lists and nobody can open, which is the
// worse failure. This way a process that dies between the commit and the delete
// leaves an ORPHAN OBJECT: bytes with no row pointing at them. A retry cannot
// finish the job either, because the row that held the storage key is gone, so
// it answers ErrNotFound.
//
// What that orphan is and is not: invisible to every read, unreachable by key
// (nothing left names it), and swept by the workspace-wide prefix delete a data
// reset performs. It is not claimed to be erased on the day it is orphaned, and
// a corpus document is deliberately not registered as PII-bearing — see the
// package doc, which states that limit rather than leaving it to be discovered.
func (s *Store) DeleteDocument(ctx context.Context, documentID ids.UUID) error {
	if err := auth.Require(ctx, "knowledge_document", principal.ActionDelete); err != nil {
		return err
	}
	if s.blob == nil {
		return ErrBlobstoreUnconfigured
	}
	var storageKey string
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		var doc deletedDocument
		if err := tx.QueryRow(ctx,
			`SELECT storage_key, filename, checksum, content_type, chunk_count
			   FROM knowledge_document WHERE id = $1 FOR UPDATE`, documentID).
			Scan(&doc.storageKey, &doc.filename, &doc.checksum, &doc.contentType, &doc.chunkCount); err != nil {
			return notFoundOr(err, "read the document to delete")
		}
		storageKey = doc.storageKey
		// The chunks go explicitly rather than by the FK's ON DELETE CASCADE.
		// The cascade would do it, but a delete that names what it destroys is
		// the one a reader of this function can check against the tombstone
		// below — and the two must agree.
		if err := deleteChunksOf(ctx, tx, documentID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM knowledge_document WHERE id = $1`, documentID); err != nil {
			return fmt.Errorf("delete the corpus document: %w", err)
		}
		// The tombstone: what was destroyed, in four keys and NOT ONE MORE.
		//
		// audit_log is append-only and outlives the erasure that clears what it
		// quotes, so a chunk of document text in here would stay readable
		// through field history, record history and the compliance log after
		// the document itself was deleted — a deletion that leaves the prose
		// behind in the audit trail is not a deletion.
		if _, err := storekit.Audit(ctx, tx, "delete", "knowledge_document", documentID,
			map[string]any{
				filenameKey:      doc.filename,
				checksumKey:      doc.checksum,
				contentTypeKey:   doc.contentType,
				chunkCountColumn: doc.chunkCount,
			}, nil); err != nil {
			return fmt.Errorf("audit corpus document delete: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}
	// Idempotent by contract, so a crash between the commit and here leaves a
	// retry able to finish the job rather than fail on bytes already gone.
	if err := s.blob.Delete(ctx, storageKey); err != nil {
		return fmt.Errorf("delete the stored document: %w", err)
	}
	return nil
}

// deletedDocument is the row read under the delete's own lock: what the
// tombstone records, and where the bytes are.
type deletedDocument struct {
	storageKey  string
	filename    string
	checksum    string
	contentType string
	chunkCount  int
}

// UnreferencedDocumentKeys answers which of these keys no knowledge_document carries.
//
// Answered here because knowledge_document is this module's table. The stored-object
// sweep holds the ledger and asks each kind's owner, since no module may read
// another's rows and the ledger sits below all of them.
func (s *Store) UnreferencedDocumentKeys(ctx context.Context, keys []string) ([]string, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	var out []string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT k FROM unnest($1::text[]) AS k
			 WHERE NOT EXISTS (SELECT 1 FROM knowledge_document d WHERE d.storage_key = k)`, keys)
		if err != nil {
			return fmt.Errorf("check which corpus document keys are unreferenced: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return fmt.Errorf("read an unreferenced corpus document key: %w", err)
			}
			out = append(out, key)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
