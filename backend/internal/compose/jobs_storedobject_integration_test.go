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
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/storedobject"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pastEveryGrace is far enough ahead that every declared kind's key is overdue.
const pastEveryGrace = 48 * time.Hour

// attachmentKind is the kind most cases reap, having the shortest grace.
const attachmentKind = "attachment"

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
	key := storedKey(e.WS, attachmentKind)
	provisionalObject(t, e, blob, key)

	reapAt(t, e, blob, time.Now().Add(2*graceOf(t, attachmentKind)))

	if !blob.deleted[key] {
		t.Error("the orphan's bytes are still in the store — an object with no row is one an erasure cannot reach")
	}
	if provisionalKeys(t, e)[key] {
		t.Error("the ledger still holds the key, so every later pass would delete bytes that are already gone")
	}
}

func TestACondemnedKeyWhoseDeleteFailedIsRetiredByTheNextPass(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	blob.refuseDelete = errors.New("the bucket said no")
	key := storedKey(e.WS, attachmentKind)
	provisionalObject(t, e, blob, key)

	reapAt(t, e, blob, time.Now().Add(2*graceOf(t, attachmentKind)))

	// The BYTES first, then the ledger. The other order forgets an object that
	// is still there, so a refused delete has to leave the key for the next pass.
	if !provisionalKeys(t, e)[key] || !condemned(t, e, key) {
		t.Fatal("a key whose delete was refused is not held condemned, so nothing would try it again")
	}

	// Inside its grace by the clock: only the condemnation lets this pass reach it.
	blob.refuseDelete = nil
	reapAt(t, e, blob, time.Now())

	if !blob.deleted[key] || provisionalKeys(t, e)[key] {
		t.Error("the next pass did not retire a condemned key, so its bytes outlive the decision to delete them")
	}
}

// TestAClaimAfterTheListingKeepsTheSourcesBytes is the race the condemnation
// closes: the reap lists an aged import source, a run claims it for staging,
// and only then does the reap reach the key.
func TestAClaimAfterTheListingKeepsTheSourcesBytes(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	source := agedImportSource(t, e, blob)
	ledger, system := reapLedger(t, e)
	orphans, err := ledger.Orphans(system, time.Now(), storedObjectReapBatch)
	if err != nil || !slices.ContainsFunc(orphans, func(o storedobject.Orphan) bool { return o.StorageKey == source }) {
		t.Fatalf("the aged source was not listed (err %v), so this case interleaves nothing", err)
	}

	if err := storedobject.Claim(e.Admin(), e.DB(), source); err != nil {
		t.Fatalf("claiming an unexpired source: %v", err)
	}
	if condemned, err := ledger.Condemn(system, time.Now(), source); err != nil || condemned {
		t.Fatalf("the reap condemned a source a run had just claimed (condemned=%v, err %v)", condemned, err)
	}
	reapAt(t, e, blob, time.Now())

	if blob.deleted[source] {
		t.Error("the reap deleted the bytes of a source a run is being staged from")
	}
}

func TestAClaimOnASourceTheReapCondemnedIsRefused(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	source := agedImportSource(t, e, blob)
	ledger, system := reapLedger(t, e)
	if condemned, err := ledger.Condemn(system, time.Now(), source); err != nil || !condemned {
		t.Fatalf("condemning an aged, unreferenced source: condemned=%v, err %v", condemned, err)
	}

	err := storedobject.Claim(e.Admin(), e.DB(), source)

	if !errors.Is(err, storedobject.ErrExpired) || !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a claim on a condemned source answered %v, want it refused as expired and not found", err)
	}
}

// graceOf is the grace a kind declared, read off the declarations the reap runs on.
func graceOf(t *testing.T, kind string) time.Duration {
	t.Helper()
	for _, ref := range StoredObjectReferences() {
		if ref.Kind == kind {
			return ref.Grace
		}
	}
	t.Fatalf("no module declares the %q kind", kind)
	return 0
}

// agedImportSource is an upload left unmapped past its grace. Aged
// in the row rather than by the reap's clock: a claim stamps the database's
// own now, and the race exists only while the two clocks agree.
func agedImportSource(t *testing.T, e *integration.Env, blob blobstore.Store) string {
	t.Helper()
	source := storedKey(e.WS, migration.ImportSourceObjectKind)
	provisionalObject(t, e, blob, source)
	age := 2 * graceOf(t, migration.ImportSourceObjectKind)
	e.WsExec(t, `UPDATE stored_object_intent SET recorded_at = now() - make_interval(secs => $2)
		WHERE storage_key = $1`, source, age.Seconds())
	return source
}

// reapLedger is the ledger the reap builds, under the principal it binds.
func reapLedger(t *testing.T, e *integration.Env) (*storedobject.Ledger, context.Context) {
	t.Helper()
	ledger, err := storedobject.NewLedger(InstallationDB(e.Pool), StoredObjectReferences()...)
	if err != nil {
		t.Fatalf("building the reap's ledger: %v", err)
	}
	return ledger, principal.SystemActing(context.Background(), "stored_object_reap_test")
}

// condemned reports whether the reap has committed to deleting key.
func condemned(t *testing.T, e *integration.Env, key string) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM stored_object_intent
		WHERE storage_key = $1 AND reaping_since IS NOT NULL`, key) > 0
}

func TestAnUploadStillInsideItsGraceIsLeftAlone(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	inflight := storedKey(e.WS, attachmentKind)
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
		"knowledge":    liveKnowledgeDocumentKey(t, e, blob),
		"offer_pdf":    liveOfferPDFKey(t, e),
	}
	for _, ref := range StoredObjectReferences() {
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
	attachment := storedKey(e.WS, attachmentKind)
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
	ours := storedKey(e.WS, attachmentKind)
	theirs := storedKey(ids.NewV7(), attachmentKind)
	provisionalObject(t, e, blob, ours)
	provisionalObject(t, e, blob, theirs)

	reapAt(t, e, blob, time.Now().Add(2*graceOf(t, attachmentKind)))

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

// liveKnowledgeDocumentKey uploads through the real document writer, which
// records and then clears the key with its row. The ingest it queues is not
// this case's subject, so the queue accepts and runs nothing.
func liveKnowledgeDocumentKey(t *testing.T, e *integration.Env, blob blobstore.Store) string {
	t.Helper()
	ctx := e.As(e.AdminUser, nil, principal.Permissions{
		RoleKeys: []string{"admin"},
		Objects: map[string]principal.ObjectGrant{
			"knowledge_corpus":   {Create: true, Read: true},
			"knowledge_document": {Create: true, Read: true},
		},
		RowScope: principal.RowScopeAll,
	})
	store := knowledge.NewStore(e.DB()).WithBlobstore(blob)
	corpus, err := store.CreateCorpus(ctx, knowledge.NewCorpus{Name: "Live corpus", TopicStatement: "what the live document covers"})
	if err != nil {
		t.Fatalf("creating the live corpus: %v", err)
	}
	doc, err := store.UploadDocument(ctx, knowledge.NewDocument{
		CorpusID: ids.UUID(corpus.Id), Filename: "live.md", ContentType: "text/markdown",
		Content: strings.NewReader("# A document somebody can open"),
	}, func(context.Context, pgx.Tx, ids.UUID) error { return nil })
	if err != nil {
		t.Fatalf("uploading the live document: %v", err)
	}
	return e.WsScalar(t, `SELECT storage_key FROM knowledge_document WHERE id = $1`, doc.Id)
}

// liveOfferPDFKey names a rendered PDF on an offer the way the render handler
// does: declared provisional, then recorded on the offer by its own writer.
func liveOfferPDFKey(t *testing.T, e *integration.Env) string {
	t.Helper()
	ctx := e.As(e.AdminUser, nil, principal.Permissions{
		RoleKeys: []string{"admin"},
		Objects: map[string]principal.ObjectGrant{
			"deal":  {Create: true, Read: true, Update: true},
			"offer": {Create: true, Read: true, Update: true},
		},
		RowScope: principal.RowScopeAll,
	})
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Live PDF deal", pipeline, open, &e.AdminUser)
	description, price, taxRate := "Retainer", int64(10000), "19.00"
	offer, err := e.Deals.CreateOffer(ctx, ids.From[ids.DealKind](deal), deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{
			Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &taxRate,
		}},
	})
	if err != nil {
		t.Fatalf("creating the live offer: %v", err)
	}
	key := storedKey(e.WS, "offer_pdf")
	if err := e.Deals.DeclarePdfProvisional(ctx, key); err != nil {
		t.Fatalf("declaring the live PDF: %v", err)
	}
	if _, _, err := e.Deals.SetPdfAssetRef(ctx, ids.From[ids.OfferKind](ids.UUID(offer.Id)), key, *offer.Version); err != nil {
		t.Fatalf("recording the live PDF on its offer: %v", err)
	}
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
