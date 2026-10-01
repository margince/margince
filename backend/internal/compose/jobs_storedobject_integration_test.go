// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The reap over real Postgres: what one pass does to the bytes and to the
// ledger, for every kind the modules declare.
//
// Real Postgres because the whole mechanism is about what survives a
// transaction that failed, and the read it rests on is a predicate assembled
// from every module's declaration — a column named wrongly anywhere fails the
// pass here and nowhere else.

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/storedobject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pastEveryGrace is far enough ahead that every declared kind's key is overdue.
const pastEveryGrace = 48 * time.Hour

// storedKey mints a key as the writers do, under the given workspace.
func storedKey(ws ids.UUID, kind string) string {
	return blobstore.WorkspaceKey(ids.From[ids.WorkspaceKind](ws), kind, ids.NewV7().String())
}

// provisionalObject stores bytes at key and declares them provisional, the way
// a writer does before its row: through the ledger's own writer.
func provisionalObject(t *testing.T, e *integration.Env, blob blobstore.Store, key string) {
	t.Helper()
	declareProvisional(t, e, key)
	if err := blob.Put(context.Background(), key, strings.NewReader("bytes nobody claimed"), 20, "application/octet-stream"); err != nil {
		t.Fatalf("storing the bytes at %s: %v", key, err)
	}
}

// declareProvisional records a key as a writer does — and, for a key whose row
// already exists, is what a clear that never ran looks like.
func declareProvisional(t *testing.T, e *integration.Env, key string) {
	t.Helper()
	if err := storedobject.Record(e.Admin(), e.DB(), key); err != nil {
		t.Fatalf("recording %s provisional: %v", key, err)
	}
}

// reapAt runs one pass with the clock at `at`.
func reapAt(t *testing.T, e *integration.Env, blob blobstore.Store, at time.Time) {
	t.Helper()
	worker := &storedObjectReapWorker{
		pool: e.Pool,
		blob: blob,
		log:  slog.New(slog.DiscardHandler),
		now:  func() time.Time { return at },
	}
	if err := worker.reap(context.Background()); err != nil {
		t.Fatalf("reap: %v", err)
	}
}

// provisionalKeys is what the ledger still holds.
func provisionalKeys(t *testing.T, e *integration.Env) map[string]bool {
	t.Helper()
	keys := map[string]bool{}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT storage_key FROM stored_object_intent`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return err
			}
			keys[key] = true
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}
	return keys
}

func TestTheReapDestroysAnOrphansBytesAndRetiresItsKey(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	key := storedKey(e.WS, "attachment")
	provisionalObject(t, e, blob, key)

	reapAt(t, e, blob, time.Now().Add(2*activities.ProvisionalObjectGrace))

	if !blob.deleted[key] {
		t.Error("the orphan's bytes are still in the store — an object with no row is one an erasure cannot reach")
	}
	if provisionalKeys(t, e)[key] {
		t.Error("the ledger still holds the key, so every later pass would delete bytes that are already gone")
	}
}

func TestAnObjectTheStoreWillNotDeleteStaysInTheLedger(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	blob.refuseDelete = errors.New("the bucket said no")
	key := storedKey(e.WS, "attachment")
	provisionalObject(t, e, blob, key)

	reapAt(t, e, blob, time.Now().Add(2*activities.ProvisionalObjectGrace))

	// The BYTES first, then the ledger. The other order forgets an object that
	// is still there, so a refused delete has to leave the key for the next pass.
	if !provisionalKeys(t, e)[key] {
		t.Error("the ledger dropped a key whose bytes are still in the store")
	}
}

func TestAnUploadStillInsideItsGraceIsLeftAlone(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	inflight := storedKey(e.WS, "attachment")
	provisionalObject(t, e, blob, inflight)

	reapAt(t, e, blob, time.Now())

	if blob.deleted[inflight] || !provisionalKeys(t, e)[inflight] {
		t.Error("an upload still inside its grace period was reaped — its row may be a statement away")
	}
}

// TestAReferencedKeyOfEveryDeclaredKindKeepsItsBytes is the second lock, asked
// of every module: a key that is BOTH provisional and named by a live row — a
// clear that never ran — survives a pass past every grace period.
func TestAReferencedKeyOfEveryDeclaredKindKeepsItsBytes(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	live := map[string]string{
		"attachment":   liveAttachmentKey(t, e, blob),
		"company_logo": liveCompanyLogoKey(t, e),
		"import":       liveImportSourceKey(t, e),
		"knowledge":    liveKnowledgeDocumentKey(t, e),
		"offer_pdf":    liveOfferPDFKey(t, e),
	}
	for _, ref := range storedObjectReferences() {
		if live[ref.Kind] == "" {
			t.Errorf("kind %q is declared and this case seeds no live row for it", ref.Kind)
		}
	}
	for _, key := range live {
		declareProvisional(t, e, key)
	}

	reapAt(t, e, blob, time.Now().Add(pastEveryGrace))

	held := provisionalKeys(t, e)
	for kind, key := range live {
		if blob.deleted[key] || !held[key] {
			t.Errorf("a %s object a live row still names was reaped — a reader can still open it", kind)
		}
	}
}

func TestAKeyOfAnUndeclaredKindIsNeverReaped(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	undeclared := storedKey(e.WS, "scratch")
	provisionalObject(t, e, blob, undeclared)

	reapAt(t, e, blob, time.Now().Add(pastEveryGrace))

	// No module said which columns name a "scratch" key, so nothing can prove
	// one unreferenced. Keeping an orphan is the safe failure; deleting a live
	// file is not.
	if blob.deleted[undeclared] || !provisionalKeys(t, e)[undeclared] {
		t.Error("a key of a kind no module declared was reaped")
	}
}

func TestAnImportSourceWaitsADayForItsColumnsToBeMapped(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	source := storedKey(e.WS, migration.ImportSourceObjectKind)
	attachment := storedKey(e.WS, "attachment")
	provisionalObject(t, e, blob, source)
	provisionalObject(t, e, blob, attachment)

	reapAt(t, e, blob, time.Now().Add(2*time.Hour))

	if blob.deleted[source] {
		t.Error("an import source two hours old was reaped while its run may still be being mapped")
	}
	// The same pass reaped the hour-grace kind, so the source was kept by its
	// own grace rather than by a pass that did nothing.
	if !blob.deleted[attachment] {
		t.Error("the attachment orphan beside it was not reaped, so this pass proves nothing")
	}
}

func TestOnePassReapsTheOrphansOfEveryWorkspace(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	ours := storedKey(e.WS, "attachment")
	theirs := storedKey(ids.NewV7(), "attachment")
	provisionalObject(t, e, blob, ours)
	provisionalObject(t, e, blob, theirs)

	reapAt(t, e, blob, time.Now().Add(2*activities.ProvisionalObjectGrace))

	if !blob.deleted[ours] || !blob.deleted[theirs] {
		t.Errorf("one pass reaped ours=%v theirs=%v, want both — the ledger names no workspace, so one pass answers for all",
			blob.deleted[ours], blob.deleted[theirs])
	}
}

// liveAttachmentKey uploads through the real writer, which records and then
// clears the key with its row.
func liveAttachmentKey(t *testing.T, e *integration.Env, blob blobstore.Store) string {
	t.Helper()
	contact := e.SeedContact(t, "Ada Live", nil)
	att, err := activities.NewStore(e.DB()).WithBlobstore(blob).UploadAttachment(e.Admin(), activities.AttachmentInput{
		EntityType: "contact", EntityID: contact, Filename: "live.pdf", ContentType: "application/pdf",
		Content: bytes.NewReader([]byte("a document somebody can open")),
	})
	if err != nil {
		t.Fatalf("uploading the live attachment: %v", err)
	}
	return e.WsScalar(t, `SELECT storage_key FROM attachment WHERE id = $1`, att.Id)
}

// liveCompanyLogoKey names a mark through the real logo writer.
func liveCompanyLogoKey(t *testing.T, e *integration.Env) string {
	t.Helper()
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Live Marks GmbH", nil))
	key := storedKey(e.WS, contacts.CompanyLogoObjectKind)
	if _, _, err := e.Contacts.SetCompanyLogo(e.Admin(), company, key, "https://example.test/logo.png"); err != nil {
		t.Fatalf("recording the live logo: %v", err)
	}
	return key
}

// liveImportSourceKey opens a run naming its source through the real run writer.
func liveImportSourceKey(t *testing.T, e *integration.Env) string {
	t.Helper()
	key := storedKey(e.WS, migration.ImportSourceObjectKind)
	ctx := e.As(e.AdminUser, nil, principal.Permissions{Objects: map[string]principal.ObjectGrant{
		"import_run": {Create: true, Read: true, Update: true},
	}})
	if _, err := migration.NewRunStore(e.DB()).CreateStagedRun(ctx, migration.CreateStagedRunInput{
		Connector: migration.ConnectorCSV, SourceRef: key, Source: "import_api",
		Mapping: migration.RunMapping{Object: migration.ObjectLead, Fields: map[string]string{"Email": "email"}, SourceKey: "Email"},
	}); err != nil {
		t.Fatalf("opening the live import run: %v", err)
	}
	return key
}

// liveKnowledgeDocumentKey inserts the document row directly: the upload path
// parses the file and queues an ingest, and this case needs only a row naming
// a key.
func liveKnowledgeDocumentKey(t *testing.T, e *integration.Env) string {
	t.Helper()
	key := storedKey(e.WS, "knowledge")
	corpus := ids.NewV7()
	e.WsExec(t, `INSERT INTO knowledge_corpus (id, name, topic_statement, captured_by)
		VALUES ($1, 'Live corpus', 'what the live document covers', 'human:fixture')`, corpus)
	e.WsExec(t, `INSERT INTO knowledge_document
		(corpus_id, filename, content_type, byte_size, storage_key, checksum, captured_by)
		VALUES ($1, 'live.txt', 'text/plain', 4, $2, 'sum', 'human:fixture')`, corpus, key)
	return key
}

// liveOfferPDFKey inserts the offer directly: the offer create path takes the
// whole pipeline, product and line-item fixture, and this case needs one row
// naming its rendered PDF.
func liveOfferPDFKey(t *testing.T, e *integration.Env) string {
	t.Helper()
	key := storedKey(e.WS, "offer_pdf")
	deal := e.SeedWonDealLinkedTo(t)
	e.WsExec(t, `INSERT INTO offer (deal_id, offer_number, currency, source, captured_by, pdf_asset_ref)
		VALUES ($1, $2, 'EUR', 'manual', 'human:fixture', $3)`, deal, "AN-"+ids.NewV7().String(), key)
	return key
}

// reapBlobstore records what the pass deleted, and can refuse.
type reapBlobstore struct {
	blobstore.Store
	refuseDelete error
	deleted      map[string]bool
}

func newReapBlobstore() *reapBlobstore {
	return &reapBlobstore{Store: blobstore.NewMemory(), deleted: map[string]bool{}}
}

func (b *reapBlobstore) Delete(ctx context.Context, key string) error {
	if b.refuseDelete != nil {
		return b.refuseDelete
	}
	if err := b.Store.Delete(ctx, key); err != nil {
		return err
	}
	b.deleted[key] = true
	return nil
}
