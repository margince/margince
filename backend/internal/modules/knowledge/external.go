// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package knowledge

// A document that lives somewhere else and is kept in step with it: an
// extension files the source's current text under the id the source gave it.
// A second filing of the same id replaces the first; the same bytes again do
// nothing.
//
// The ingest is not touched. A replacement swaps the row's bytes and puts it back
// to queued in ONE transaction, so the ingest that follows is an ordinary first
// attempt, and its idempotent-retry protocol (BeginIngest deletes the earlier
// passages) is what swaps the passages.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/auditentity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// GDocsSource is the managed_source of the corpus the Google Docs extension
// files into. The schema's CHECK holds the same set.
const GDocsSource = "gdocs"

// The bounds on what a source may say about a document. The meta is shown beside
// a citation, so it is kept short and flat.
// MetaKeys is the closed set of fields a source may state about a document.
// "missing" lists, comma separated, the stated fields the source did not carry,
// so a reader sees the gap rather than a guess. "path" is the folders the source
// keeps the document under; the source states it, a document's own header cannot.
var MetaKeys = []string{"owner", "department", "country", "type", "effective", "review", "audience", "version", "path", "missing"}

const (
	maxExternalID    = 256
	maxMetaKeys      = 16
	maxMetaValueRune = 500
)

// ManagedCorpus names the corpus a source files into, for the first filing.
type ManagedCorpus struct {
	Source string
	Name   string
	Topic  string
}

// ExternalDocument is one source document as an extension hands it over.
type ExternalDocument struct {
	ExternalID  string
	Filename    string
	ContentType string
	Content     io.ReadSeeker
	Meta        map[string]string
}

// ExternalOutcome says what a filing did.
type ExternalOutcome string

// The three things a filing can do.
const (
	ExternalCreated   ExternalOutcome = "created"
	ExternalReplaced  ExternalOutcome = "replaced"
	ExternalUnchanged ExternalOutcome = "unchanged"
)

// IngestBusyError is a replacement refused while the earlier version is being
// read: an ingest that already started would finish on the old bytes and mark
// the document done, and the queued one for the new bytes would be folded into
// it. The caller tries again later.
type IngestBusyError struct{ Filename string }

func (e *IngestBusyError) Error() string {
	return fmt.Sprintf("%q is still being read; try the replacement again shortly", e.Filename)
}

func validateExternal(in *ExternalDocument) error {
	if in.ExternalID == "" {
		return fmt.Errorf("%w: the external id is empty", apperrors.ErrInvalidArgument)
	}
	if err := httperr.RequireWithin("external_id", in.ExternalID, maxExternalID); err != nil {
		return err
	}
	if len(in.Meta) > maxMetaKeys {
		return fmt.Errorf("%w: at most %d meta fields", apperrors.ErrInvalidArgument, maxMetaKeys)
	}
	for k, v := range in.Meta {
		if !slices.Contains(MetaKeys, k) {
			return fmt.Errorf("%w: %q is not a meta field; the fields are %s",
				apperrors.ErrInvalidArgument, extension.SafeFilename(k, 0), strings.Join(MetaKeys, ", "))
		}
		if err := httperr.RequireWithin("meta "+k, v, maxMetaValueRune); err != nil {
			return err
		}
		// A value is shown beside a citation, so it may not carry what rewrites
		// the line it sits on: a control character, a newline, or a format
		// character such as a bidirectional override or isolate. Refused rather
		// than stripped, so the source's text is never silently altered.
		if i := strings.IndexFunc(v, func(r rune) bool {
			return unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp)
		}); i >= 0 {
			return fmt.Errorf("%w: meta %s holds a control or direction-changing character", apperrors.ErrInvalidArgument, k)
		}
	}
	return nil
}

// UpsertExternal files the document in the corpus its source manages, creating
// the corpus on first use. It replaces the live document with the same
// ExternalID, and does nothing when that document already holds these bytes.
func (s *Store) UpsertExternal(
	ctx context.Context, corpus ManagedCorpus, in ExternalDocument, queue QueueIngest,
) (crmcontracts.KnowledgeDocument, ExternalOutcome, error) {
	var none crmcontracts.KnowledgeDocument
	if err := auth.Require(ctx, "knowledge_document", principal.ActionCreate); err != nil {
		return none, "", err
	}
	if s.blob == nil {
		return none, "", ErrBlobstoreUnconfigured
	}
	if err := validateExternal(&in); err != nil {
		return none, "", err
	}
	in.Filename = extension.SafeFilename(in.Filename, 0)
	media, ok := acceptedType(in.ContentType, in.Filename)
	if !ok {
		return none, "", &UnsupportedTypeError{Got: clampEchoedType(media)}
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return none, "", err
	}
	checksum, size, err := blobstore.Digest(in.Content)
	if err != nil {
		return none, "", err
	}

	var corpusID ids.UUID
	var unchanged bool
	if err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		if corpusID, err = ensureManagedCorpus(ctx, tx, corpus, by); err != nil {
			return err
		}
		unchanged, err = holdsBytes(ctx, tx, corpusID, in.ExternalID, checksum)
		return err
	}); err != nil {
		return none, "", err
	}
	if unchanged {
		var doc crmcontracts.KnowledgeDocument
		err := s.tx(ctx, func(tx pgx.Tx) (err error) {
			doc, err = readExternal(ctx, tx, corpusID, in.ExternalID, s.embedIdentity())
			return err
		})
		return doc, ExternalUnchanged, err
	}

	id := ids.NewV7()
	key := blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](storekit.MustWorkspace(ctx)),
		string(storedobjects.KindKnowledge), id.String())
	if err := storedobjects.NewLedger(s.db).Record(ctx, storedobjects.KindKnowledge, key); err != nil {
		return none, "", err
	}
	if err := s.blob.Put(ctx, key, in.Content, size, media); err != nil {
		return none, "", fmt.Errorf("store the external document: %w", err)
	}

	var out crmcontracts.KnowledgeDocument
	var outcome ExternalOutcome
	var oldKey string
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// The corpus row first, as WriteChunks does: two filings of one source id
		// then serialize here instead of both finding no row and both inserting.
		if _, err := storekit.LockRow(ctx, tx, "knowledge_corpus", corpusID, storekit.LiveOnly); err != nil {
			return err
		}
		var err error
		oldKey, outcome, err = fileExternal(ctx, tx, externalFiling{
			corpusID: corpusID, id: id, key: key, media: media, checksum: checksum, size: size,
			by: by, source: corpus.Source, in: in,
		}, queue)
		if err != nil {
			return err
		}
		if err := storedobjects.Clear(ctx, tx, key); err != nil {
			return err
		}
		out, err = readExternal(ctx, tx, corpusID, in.ExternalID, s.embedIdentity())
		return err
	})
	if err != nil {
		// Nothing names the new bytes: the write failed. Same cleanup the plain
		// upload does.
		return none, "", errors.Join(err, s.dropUnreferenced(ctx, key))
	}
	if outcome == ExternalUnchanged {
		// Lost a race to an identical filing; the ledger entry is already cleared.
		if err := s.blob.Delete(ctx, key); err != nil {
			return out, outcome, fmt.Errorf("delete the duplicate stored document: %w", err)
		}
		return out, outcome, nil
	}
	if oldKey != "" {
		if err := s.blob.Delete(ctx, oldKey); err != nil {
			return out, outcome, fmt.Errorf("delete the replaced stored document: %w", err)
		}
	}
	return out, outcome, nil
}

type externalFiling struct {
	corpusID ids.UUID
	id       ids.UUID
	key      string
	media    string
	checksum string
	size     int64
	by       string
	source   string
	in       ExternalDocument
}

// fileExternal inserts or replaces, inside the transaction that holds the
// corpus lock. It returns the storage key a replacement left behind, to be
// deleted once the transaction commits.
func fileExternal(ctx context.Context, tx pgx.Tx, f externalFiling, queue QueueIngest) (string, ExternalOutcome, error) {
	var docID ids.UUID
	var oldKey, oldSum, oldStatus, oldName string
	err := tx.QueryRow(ctx,
		`SELECT id, storage_key, checksum, ingest_status, filename FROM knowledge_document
		  WHERE corpus_id = $1 AND external_id = $2 AND archived_at IS NULL FOR UPDATE`,
		f.corpusID, f.in.ExternalID).Scan(&docID, &oldKey, &oldSum, &oldStatus, &oldName)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		if _, err := tx.Exec(ctx,
			`INSERT INTO knowledge_document
			   (id, corpus_id, filename, content_type, byte_size, storage_key, checksum,
			    managed_source, external_id, meta, captured_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			f.id, f.corpusID, f.in.Filename, f.media, f.size, f.key, f.checksum,
			f.source, f.in.ExternalID, metaOrNil(f.in.Meta), f.by); err != nil {
			if storekit.IsUniqueViolation(err) {
				return "", "", &AlreadyFiledError{}
			}
			return "", "", fmt.Errorf("insert external document: %w", err)
		}
		docID = f.id
		return "", ExternalCreated, auditAndQueue(ctx, tx, "create", docID, nil, f, queue)
	case err != nil:
		return "", "", fmt.Errorf("read the external document to replace: %w", err)
	}
	if oldSum == f.checksum && oldStatus != "failed" {
		return "", ExternalUnchanged, nil
	}
	if oldStatus == "running" {
		return "", "", &IngestBusyError{Filename: oldName}
	}
	if err := deleteChunksOf(ctx, tx, docID); err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE knowledge_document
		    SET filename = $2, content_type = $3, byte_size = $4, storage_key = $5, checksum = $6,
		        meta = $7, ingest_status = 'queued', ingest_detail = NULL, chunk_count = 0,
		        ingest_started_at = NULL
		  WHERE id = $1`,
		docID, f.in.Filename, f.media, f.size, f.key, f.checksum, metaOrNil(f.in.Meta)); err != nil {
		if storekit.IsUniqueViolation(err) {
			return "", "", &AlreadyFiledError{}
		}
		return "", "", fmt.Errorf("replace external document: %w", err)
	}
	// A failed row's object is already gone; deleting a missing key is a no-op.
	return oldKey, ExternalReplaced, auditAndQueue(ctx, tx, "update", docID,
		map[string]any{filenameKey: oldName, checksumKey: oldSum}, f, queue)
}

func auditAndQueue(ctx context.Context, tx pgx.Tx, op string, docID ids.UUID, before map[string]any, f externalFiling, queue QueueIngest) error {
	// The image names the source and the bytes' identity, never the text, for
	// the reason DeleteDocument's tombstone gives.
	if _, err := storekit.Audit(ctx, tx, op, auditentity.KnowledgeDocument, docID, before, map[string]any{
		corpusIDKey:      f.corpusID,
		filenameKey:      f.in.Filename,
		checksumKey:      f.checksum,
		contentTypeKey:   f.media,
		byteSizeKey:      f.size,
		managedSourceKey: f.source,
	}); err != nil {
		return fmt.Errorf("audit external document %s: %w", op, err)
	}
	if err := queue(ctx, tx, docID); err != nil {
		return fmt.Errorf("queue the external document's ingest: %w", err)
	}
	return nil
}

// dropUnreferenced deletes bytes no row names, then retires their declaration.
func (s *Store) dropUnreferenced(ctx context.Context, key string) error {
	if err := s.blob.Delete(ctx, key); err != nil {
		return fmt.Errorf("delete the stored object after a refused filing: %w", err)
	}
	return s.tx(ctx, func(tx pgx.Tx) error { return storedobjects.Clear(ctx, tx, key) })
}

func metaOrNil(m map[string]string) any {
	if len(m) == 0 {
		return nil
	}
	return m
}

func holdsBytes(ctx context.Context, tx pgx.Tx, corpusID ids.UUID, externalID, checksum string) (bool, error) {
	var same bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM knowledge_document
		  WHERE corpus_id = $1 AND external_id = $2 AND archived_at IS NULL
		    AND checksum = $3 AND ingest_status <> 'failed')`,
		corpusID, externalID, checksum).Scan(&same); err != nil {
		return false, fmt.Errorf("read whether the external document already holds these bytes: %w", err)
	}
	return same, nil
}

func readExternal(ctx context.Context, tx pgx.Tx, corpusID ids.UUID, externalID, identity string) (crmcontracts.KnowledgeDocument, error) {
	var id ids.UUID
	if err := tx.QueryRow(ctx,
		`SELECT id FROM knowledge_document WHERE corpus_id = $1 AND external_id = $2 AND archived_at IS NULL`,
		corpusID, externalID).Scan(&id); err != nil {
		return crmcontracts.KnowledgeDocument{}, notFoundOr(err, "read the external document")
	}
	return readDocument(ctx, tx, id, identity)
}

// ensureManagedCorpus finds the source's live corpus, creating it the first time.
// default_ask stays false: becoming the palette's default is an administrator's
// decision, not a sync's.
func ensureManagedCorpus(ctx context.Context, tx pgx.Tx, c ManagedCorpus, by string) (ids.UUID, error) {
	var id ids.UUID
	for range 2 {
		err := tx.QueryRow(ctx,
			`SELECT id FROM knowledge_corpus WHERE managed_source = $1 AND archived_at IS NULL`, c.Source).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return ids.Nil, fmt.Errorf("read the managed corpus: %w", err)
		}
		newID := ids.NewV7()
		tag, err := tx.Exec(ctx,
			`INSERT INTO knowledge_corpus (id, name, topic_statement, managed_source, captured_by)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (managed_source) WHERE managed_source IS NOT NULL AND archived_at IS NULL DO NOTHING`,
			newID, c.Name, c.Topic, c.Source, by)
		if err != nil {
			return ids.Nil, fmt.Errorf("create the managed corpus: %w", err)
		}
		if tag.RowsAffected() == 1 {
			if _, err := storekit.Audit(ctx, tx, "create", "knowledge_corpus", newID, nil, map[string]any{
				"name": c.Name, "topic_statement": c.Topic, managedSourceKey: c.Source,
			}); err != nil {
				return ids.Nil, fmt.Errorf("audit the managed corpus: %w", err)
			}
			return newID, nil
		}
	}
	return ids.Nil, errors.New("the managed corpus could not be created or found")
}

// DeleteExternal removes the source's document with this id, with its passages
// and stored file. A document already gone is not an error: the source deleted
// it and the sync is reporting that, possibly twice.
func (s *Store) DeleteExternal(ctx context.Context, source, externalID string) error {
	var id ids.UUID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT d.id FROM knowledge_document d JOIN knowledge_corpus c ON c.id = d.corpus_id
			  WHERE c.managed_source = $1 AND c.archived_at IS NULL
			    AND d.external_id = $2 AND d.archived_at IS NULL`, source, externalID).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Require(ctx, "knowledge_document", principal.ActionDelete)
	}
	if err != nil {
		return fmt.Errorf("find the external document to delete: %w", err)
	}
	return s.DeleteDocument(ctx, id)
}

// ExternalIDs lists the source ids of the live documents a source has filed.
func (s *Store) ExternalIDs(ctx context.Context, source string) ([]string, error) {
	if err := auth.Require(ctx, "knowledge_document", principal.ActionRead); err != nil {
		return nil, err
	}
	out := []string{}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT d.external_id FROM knowledge_document d JOIN knowledge_corpus c ON c.id = d.corpus_id
			  WHERE c.managed_source = $1 AND c.archived_at IS NULL
			    AND d.external_id IS NOT NULL AND d.archived_at IS NULL
			  ORDER BY d.external_id`, source)
		if err != nil {
			return fmt.Errorf("list the external document ids: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			out = append(out, id)
		}
		return rows.Err()
	})
	return out, err
}
