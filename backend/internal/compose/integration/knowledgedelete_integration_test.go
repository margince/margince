// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The delete suite: what a hard delete must reach, and what it must not leave
// behind in the audit trail.
//
// A vector is the document's text in another shape — a similarity probe
// reconstructs neighbourhoods of what it was made from — so a delete that left
// vectors, or the stored original, would be decorative.

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/knowledge"
	"github.com/gradionhq/margince/backend/internal/platform/blobstore"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// wsText reads one text scalar in a workspace-bound transaction. The shared
// harness offers WsCount and WsExec but no string read, and this suite needs
// three: a storage key and two audit images.
func wsText(t *testing.T, e *Env, sql string, args ...any) string {
	t.Helper()
	var out string
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), sql, args...).Scan(&out)
	}); err != nil {
		t.Fatalf("read: %v", err)
	}
	return out
}

// wsTexts reads a column of text scalars the same way.
func wsTexts(t *testing.T, e *Env, sql string, args ...any) []string {
	t.Helper()
	var out []string
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		rows, err := tx.Query(e.Admin(), sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("read: %v", err)
	}
	return out
}

func storageKeyOf(t *testing.T, e *Env, documentID ids.UUID) string {
	t.Helper()
	return wsText(t, e, `SELECT storage_key FROM knowledge_document WHERE id = $1`, documentID)
}

// The blob goes with the row.
func TestDeletingADocumentRemovesItsChunksItsVectorsAndItsBlob(t *testing.T) {
	ee := newEmbedEnv(t)
	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("embed: %v", err)
	}
	if embeddedCount(t, ee.env, ee.corpus) == 0 {
		t.Fatal("nothing was embedded, so this delete would prove nothing")
	}
	key := storageKeyOf(t, ee.env, ee.doc)

	if err := ee.store.DeleteDocument(ee.ctx, ee.doc); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if live := liveChunkCount(t, ee.env, ee.corpus); live != 0 {
		t.Fatalf("%d passage(s) survived the delete", live)
	}
	// Every row, not only the live ones: this is a delete, not an archive.
	if any := ee.env.WsCount(t,
		`SELECT count(*) FROM knowledge_chunk WHERE corpus_id = $1`, ee.corpus); any != 0 {
		t.Fatalf("%d passage row(s) survived the delete", any)
	}
	if rows := ee.env.WsCount(t,
		`SELECT count(*) FROM knowledge_document WHERE id = $1`, ee.doc); rows != 0 {
		t.Fatal("the document row survived the delete")
	}
	if _, _, err := ee.blob.Get(ee.ctx, key); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("the stored file survived the delete: %v", err)
	}
}

// No object store is a refusal, not a silent partial delete. A screen saying
// the document is gone while the file it was made from is still there — with
// nothing left pointing at it — is the worst available outcome.
func TestDeletingWithNoObjectStoreConfiguredRefuses(t *testing.T) {
	ee := newEmbedEnv(t)
	storeless := knowledge.NewStore(ee.env.DB())

	if err := storeless.DeleteDocument(ee.ctx, ee.doc); !errors.Is(err, knowledge.ErrBlobstoreUnconfigured) {
		t.Fatalf("delete with no object store = %v, want ErrBlobstoreUnconfigured", err)
	}
	// Nothing was destroyed on the way to the refusal.
	if rows := ee.env.WsCount(t,
		`SELECT count(*) FROM knowledge_document WHERE id = $1`, ee.doc); rows != 1 {
		t.Fatal("the refused delete removed the document row anyway")
	}
	if live := liveChunkCount(t, ee.env, ee.corpus); live == 0 {
		t.Fatal("the refused delete removed the passages anyway")
	}
}

func TestADeletedDocumentsPassagesAreNeverRetrievedAgain(t *testing.T) {
	ae := newAskEnv(t)
	ae.embedder.vectors["how is a message filed"] = []float32{1, 0, 0, 0}
	if _, passages := ae.ask(t, "how is a message filed"); len(passages) == 0 {
		t.Fatal("nothing was retrievable before the delete, so this proves nothing")
	}

	if err := ae.store.DeleteDocument(ae.ctx, ae.doc); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, passages := ae.ask(t, "how is a message filed")
	if len(passages) != 0 {
		t.Fatalf("a deleted document's passages were retrieved: %d", len(passages))
	}
}

// The image is filename, checksum, content_type, chunk_count and nothing else.
//
// audit_log.before/after outlive the erasure that clears what they quote, so a
// chunk of document text in here would be readable through field history,
// record history and the compliance log after the document was deleted.
// Widening an audit image has silently changed authorization outcomes in this
// tree before — if this test fails, argue with it rather than editing it.
func TestTheDocumentAuditImageCarriesNoDocumentText(t *testing.T) {
	ee := newEmbedEnv(t)
	// The passage text is distinctive, so its presence anywhere in the trail is
	// unambiguous rather than a substring coincidence.
	marker := "Margince files a captured message against the account it belongs to"
	doc := ee.upload(t, "distinctive.md", "text/markdown", marker)
	ee.ingest(t, doc)
	if err := ee.store.DeleteDocument(ee.ctx, doc); err != nil {
		t.Fatalf("delete: %v", err)
	}

	images := wsTexts(t, ee.env,
		`SELECT coalesce(before::text, '') || coalesce(after::text, '')
		   FROM audit_log WHERE entity_type = 'knowledge_document' AND entity_id = $1`, doc)
	if len(images) == 0 {
		t.Fatal("the delete wrote no audit row at all")
	}
	for _, image := range images {
		if strings.Contains(image, marker) {
			t.Fatalf("a document audit image carries the document's own text: %s", image)
		}
	}
	// And the delete's own image carries exactly the four keys it is allowed.
	deleteImage := wsText(t, ee.env,
		`SELECT before::text FROM audit_log
		  WHERE entity_type = 'knowledge_document' AND entity_id = $1 AND action = 'delete'`, doc)
	var keys map[string]any
	if err := json.Unmarshal([]byte(deleteImage), &keys); err != nil {
		t.Fatalf("the delete image is not an object: %v", err)
	}
	want := map[string]bool{"filename": true, "checksum": true, "content_type": true, "chunk_count": true}
	for k := range keys {
		if !want[k] {
			t.Fatalf("the delete image carries %q, which is not one of the four keys it is pinned to", k)
		}
	}
	if len(keys) != len(want) {
		t.Fatalf("the delete image carries %d keys, want the four it is pinned to: %v", len(keys), keys)
	}
}

// Deleting twice is absent, not a second tombstone.
func TestDeletingADocumentTwiceIsNotFound(t *testing.T) {
	ee := newEmbedEnv(t)
	if err := ee.store.DeleteDocument(ee.ctx, ee.doc); err != nil {
		t.Fatalf("first delete: %v", err)
	}
	if err := ee.store.DeleteDocument(ee.ctx, ee.doc); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}

// A rep may not delete: the delete is hard and takes a third party's uploaded
// prose with it, which is admin/ops authority.
func TestARepMayNotDeleteADocument(t *testing.T) {
	ee := newEmbedEnv(t)
	rep := ee.env.As(ee.env.Rep1, nil, corpusRepPerms)

	if err := ee.store.DeleteDocument(rep, ee.doc); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("rep delete = %v, want ErrPermissionDenied", err)
	}
	if rows := ee.env.WsCount(t,
		`SELECT count(*) FROM knowledge_document WHERE id = $1`, ee.doc); rows != 1 {
		t.Fatal("a refused delete removed the row anyway")
	}
}
