// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// A document arriving in a corpus: the bytes go to object storage, the row
// records where, and the ingest that turns it into chunks is queued behind it.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/platform/blobstore"
	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
	"github.com/gradionhq/margince/backend/pkg/extension"
)

// acceptedContentTypes is the whole list, and it is short for one reason: this
// product has no document parser. A type is here only when the file's bytes ARE
// its text, so accepting a PDF would mean ingesting its binary envelope as
// prose — a corpus that answers nothing while reporting a successful upload.
//
// The refusal names this list, because "unsupported" alone leaves the uploader
// with nothing to do.
var acceptedContentTypes = []string{
	"text/plain",
	"text/markdown",
	"text/csv",
	"application/json",
}

// UnsupportedTypeError is a file whose bytes are not its own text. It carries
// no field name: the refusal is about the file, and the transport maps it to
// 415 rather than to a validation error on a form field.
type UnsupportedTypeError struct {
	Got string
}

func (e *UnsupportedTypeError) Error() string {
	return fmt.Sprintf("%s cannot be read as text; the accepted types are %s",
		e.Got, strings.Join(acceptedContentTypes, ", "))
}

// ErrBlobstoreUnconfigured is an upload arriving at an installation with no
// object storage bound. It is the deployment's fault, not the caller's, and
// answering it as a validation failure would send them off to fix their file.
var ErrBlobstoreUnconfigured = errors.New("knowledge: object storage is not configured; a document cannot be stored")

// acceptedType normalises a browser's Content-Type (which carries parameters —
// `text/markdown; charset=utf-8`) down to the media type the list names.
func acceptedType(contentType string) (string, bool) {
	media := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	media = strings.ToLower(media)
	return media, slices.Contains(acceptedContentTypes, media)
}

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
	if err := auth.Require(ctx, "knowledge_document", principal.ActionCreate); err != nil {
		return crmcontracts.KnowledgeDocument{}, err
	}
	if s.blob == nil {
		return crmcontracts.KnowledgeDocument{}, ErrBlobstoreUnconfigured
	}
	media, ok := acceptedType(in.ContentType)
	if !ok {
		return crmcontracts.KnowledgeDocument{}, &UnsupportedTypeError{Got: media}
	}
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		_, err := readCorpus(ctx, tx, in.CorpusID, storekit.LiveOnly)
		return err
	}); err != nil {
		return crmcontracts.KnowledgeDocument{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.KnowledgeDocument{}, err
	}

	id := ids.NewV7()
	key := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](storekit.MustWorkspace(ctx)), "knowledge", id.String())
	// The name a person TYPED, made safe before it reaches the column — the
	// same call every other uploaded name in this tree goes through. It is
	// presentational only (nothing opens a file by it) but it is read back in a
	// citation, a list and an ingest failure, and a path separator or a
	// bidirectional override in it rewrites whichever of those quotes it.
	in.Filename = extension.SafeFilename(in.Filename, 0)

	checksum, size, err := digestDocument(in.Content)
	if err != nil {
		return crmcontracts.KnowledgeDocument{}, err
	}
	if err := s.blob.Put(ctx, key, in.Content, size, media); err != nil {
		return crmcontracts.KnowledgeDocument{}, fmt.Errorf("store the corpus document: %w", err)
	}

	var out crmcontracts.KnowledgeDocument
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// Re-read under the write's own transaction: a corpus archived while
		// the bytes were being stored would otherwise take a live document.
		if _, err := readCorpus(ctx, tx, in.CorpusID, storekit.LiveOnly); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO knowledge_document
			   (id, corpus_id, filename, content_type, byte_size, storage_key, checksum, captured_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			id, in.CorpusID, in.Filename, media, size, key, checksum, by); err != nil {
			return fmt.Errorf("insert corpus document: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "create", "knowledge_document", id, nil, map[string]any{
			"corpus_id":    in.CorpusID,
			"filename":     in.Filename,
			"content_type": media,
			"byte_size":    size,
		}); err != nil {
			return fmt.Errorf("audit corpus document create: %w", err)
		}
		// Queued INSIDE the row's transaction: a job enqueued outside it can
		// wake before the row exists, or survive a rollback that leaves it
		// pointing at nothing.
		if err := queue(ctx, tx, id); err != nil {
			return fmt.Errorf("queue the corpus document's ingest: %w", err)
		}
		var rerr error
		out, rerr = readDocument(ctx, tx, id)
		return rerr
	})
	return out, err
}

// digestDocument hashes the upload and measures it, rewinding so the same
// reader can then be streamed to storage.
func digestDocument(content io.ReadSeeker) (string, int64, error) {
	sum := sha256.New()
	size, err := io.Copy(sum, content)
	if err != nil {
		return "", 0, fmt.Errorf("knowledge: reading the uploaded document: %w", err)
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return "", 0, fmt.Errorf("knowledge: rewinding the uploaded document: %w", err)
	}
	return hex.EncodeToString(sum.Sum(nil)), size, nil
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
// The blob goes LAST, after the rows commit. The other order deletes bytes that
// a failed transaction then leaves a live row promising. This way a failure
// leaves at worst an orphan object — recoverable, and invisible to every read —
// rather than a row whose document cannot be opened.
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
				"filename":       doc.filename,
				"checksum":       doc.checksum,
				"content_type":   doc.contentType,
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
