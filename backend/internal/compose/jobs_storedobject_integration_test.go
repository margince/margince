// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The reap itself: what the pass does to the bytes and to the ledger.
//
// The ledger's reads are proved where they live (activities). This is the other
// half of the issue's acceptance — "a transaction that fails after a put leaves
// no object behind ONCE THE CLEANUP JOB HAS RUN" — which is a claim about the
// pass, and about the order it does its two deletes in.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// orphanedObject plants an object and the ledger row that says it is
// provisional, dated past the grace period.
func orphanedObject(t *testing.T, e *integration.Env, blob blobstore.Store, key string) {
	t.Helper()
	orphanedObjectOfKind(t, e, blob, key, storedobjects.KindAttachment)
}

// orphanedObjectOfKind plants one under the kind a given writer would declare.
func orphanedObjectOfKind(
	t *testing.T, e *integration.Env, blob blobstore.Store, key string, kind storedobjects.Kind,
) {
	t.Helper()
	ctx := context.Background()
	if err := blob.Put(ctx, key, strings.NewReader("bytes nobody claimed"), 20, "application/octet-stream"); err != nil {
		t.Fatalf("storing the orphan's bytes: %v", err)
	}
	if err := database.WithWorkspaceTx(e.As(e.Rep1, nil, integration.AdminPerms), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO stored_object_intent (storage_key, kind, recorded_at)
			VALUES ($1, $2, now() - $3::interval)`,
			key, string(kind), (2 * storedobjects.ProvisionalObjectGrace).String())
		return err
	}); err != nil {
		t.Fatalf("planting the provisional key: %v", err)
	}
}

// provisionalKeys is what the ledger still holds.
func provisionalKeys(t *testing.T, e *integration.Env) []string {
	t.Helper()
	var keys []string
	if err := database.WithWorkspaceTx(e.As(e.Rep1, nil, integration.AdminPerms), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT storage_key FROM stored_object_intent ORDER BY storage_key`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return err
			}
			keys = append(keys, key)
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
	const key = "reap/attachment/orphan"
	orphanedObject(t, e, blob, key)

	worker := &storedObjectReapWorker{pool: e.Pool, blob: blob, log: slog.New(slog.DiscardHandler), now: time.Now}
	if err := worker.reapWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("reapWorkspace: %v", err)
	}

	if !blob.deleted[key] {
		t.Error("the orphan's bytes are still in the store — an object with no row is one an erasure cannot reach")
	}
	if got := provisionalKeys(t, e); len(got) != 0 {
		t.Errorf("the ledger still holds %v, so every later pass would delete bytes that are already gone", got)
	}
}

func TestAnObjectTheStoreWillNotDeleteStaysInTheLedger(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	blob.refuseDelete = errors.New("the bucket said no")
	const key = "reap/attachment/stubborn"
	orphanedObject(t, e, blob, key)

	worker := &storedObjectReapWorker{pool: e.Pool, blob: blob, log: slog.New(slog.DiscardHandler), now: time.Now}
	if err := worker.reapWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("reapWorkspace: %v — one key that will not delete must not stop the pass", err)
	}

	// The BYTES first, then the ledger. The other order forgets an object that
	// is still there, which is the state the whole ledger exists to make
	// impossible — so a refused delete has to leave the key for the next pass.
	if got := provisionalKeys(t, e); len(got) != 1 || got[0] != key {
		t.Errorf("the ledger holds %v, want the key whose bytes are still in the store", got)
	}
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

var _ io.Reader = strings.NewReader("")

// Every declared kind is swept, each adjudicated by the module that owns its table.
//
// One orphan per kind, all expected collected: a kind the sweep cannot adjudicate is
// bytes that stay forever while the count reports a clean pass, and asking one
// module's table about another's keys answers "unreferenced" for a live file.
func TestTheSweepCollectsAnOrphanOfEveryDeclaredKind(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	planted := map[storedobjects.Kind]string{}
	for _, kind := range storedobjects.AllKinds() {
		key := "ws/" + string(kind) + "/orphan-" + string(kind)
		orphanedObjectOfKind(t, e, blob, key, kind)
		planted[kind] = key
	}
	// The baseline the assertion below needs: the ledger holds one key per kind
	// before the sweep, so an empty ledger afterwards is the sweep's doing and not a
	// plant that never landed.
	if len(storedobjects.AllKinds()) == 0 {
		t.Fatal("storedobjects.AllKinds() is empty, so this test plants nothing and sweeps nothing: " +
			"both the loop above and the count below read that slice, and an empty one agrees with itself")
	}
	if held := provisionalKeys(t, e); len(held) != len(storedobjects.AllKinds()) {
		t.Fatalf("the ledger holds %d keys before the sweep, want one per kind (%d)",
			len(held), len(storedobjects.AllKinds()))
	}

	worker := &storedObjectReapWorker{pool: e.Pool, blob: blob, log: slog.New(slog.DiscardHandler), now: time.Now}
	if err := worker.reapWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("reapWorkspace: %v", err)
	}

	for kind, key := range planted {
		if !blob.deleted[key] {
			t.Errorf("a %s orphan was left in the bucket: its kind has no owner the sweep can ask, "+
				"so the bytes stay where no erasure reaches them", kind)
		}
	}
	if held := provisionalKeys(t, e); len(held) != 0 {
		t.Errorf("the ledger still holds %v after the sweep", held)
	}
}

// A key still inside its grace period survives a pass, bytes and all.
//
// The cutoff is the only thing standing between the reaper and an upload still in
// flight: the window between a put and its row is a handful of statements, and a
// sweep that did not wait would delete the object of a file somebody is still
// uploading. That is a worse failure than the orphan it chases, and nothing about it
// shows up in a test that only plants keys already past the grace.
func TestAKeyStillInsideItsGracePeriodSurvivesAPass(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	const key = "ws/attachment/inflight"
	if err := blob.Put(context.Background(), key, strings.NewReader("bytes mid-upload"), 16,
		"application/octet-stream"); err != nil {
		t.Fatalf("storing the in-flight bytes: %v", err)
	}
	// Recorded through the LEDGER, not an insert of this test's own: the stamp the
	// cutoff is then read against is the one a writer actually leaves, so a writer
	// that stopped stamping it would fail here rather than pass against a row this
	// test wrote correctly on its behalf.
	if err := storedobjects.NewLedger(InstallationDB(e.Pool)).Record(
		e.As(e.Rep1, nil, integration.AdminPerms), storedobjects.KindAttachment, key); err != nil {
		t.Fatalf("recording the in-flight key: %v", err)
	}

	worker := &storedObjectReapWorker{pool: e.Pool, blob: blob, log: slog.New(slog.DiscardHandler), now: time.Now}
	if err := worker.reapWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("reapWorkspace: %v", err)
	}

	if blob.deleted[key] {
		t.Error("an upload still inside its grace period had its bytes deleted")
	}
	held := provisionalKeys(t, e)
	if len(held) != 1 || held[0] != key {
		t.Errorf("the ledger holds %v, want the in-flight key still declared: retired early, the "+
			"next pass has nothing to protect", held)
	}
}

// A key its owning table carries survives the sweep, for every kind.
//
// THE SECOND LOCK, and the half the orphan tests above cannot show. The first lock is
// that a key was declared at all; this one is that a declaration still standing over
// a LIVE row is a clear that never ran, not an orphan. Without it the reaper deletes
// the bytes of a file somebody can still open — and the pass reports a clean success,
// because deleting what the ledger offered is exactly what a healthy sweep does.
//
// The declaration is retired either way, which is the point of telling them apart:
// the bytes stay and the stale row goes, so it stops occupying a slot in every later
// pass's limit.
func TestAReferencedObjectOfEveryKindSurvivesTheSweep(t *testing.T) {
	e := integration.Setup(t)
	blob := newReapBlobstore()
	live := map[storedobjects.Kind]string{}
	for _, kind := range storedobjects.AllKinds() {
		key := "ws/" + string(kind) + "/live-" + string(kind)
		orphanedObjectOfKind(t, e, blob, key, kind)
		referenceKeyFromItsOwner(t, e, kind, key)
		live[kind] = key
	}
	if len(live) == 0 {
		t.Fatal("no kinds were planted, so this test asserts nothing about any of them")
	}

	worker := &storedObjectReapWorker{pool: e.Pool, blob: blob, log: slog.New(slog.DiscardHandler), now: time.Now}
	if err := worker.reapWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("reapWorkspace: %v", err)
	}

	for kind, key := range live {
		if blob.deleted[key] {
			t.Errorf("a %s object a live row references was destroyed: its owner was asked and "+
				"the answer was taken for an orphan", kind)
		}
	}
	// The declarations go, because a referenced key still declared is a clear that
	// never ran: left standing it is re-read on every pass for ever.
	if held := provisionalKeys(t, e); len(held) != 0 {
		t.Errorf("the ledger still holds %v, so every later pass spends part of its limit "+
			"re-adjudicating keys it has already settled", held)
	}
}

// referenceKeyFromItsOwner writes the row that speaks for one key, in the table the
// sweep asks about that kind.
func referenceKeyFromItsOwner(t *testing.T, e *integration.Env, kind storedobjects.Kind, key string) {
	t.Helper()
	switch kind {
	case storedobjects.KindAttachment:
		activityID := ids.NewV7()
		e.WsExec(t, `
			INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
			VALUES ($1, 'note', 'a note with a file', now(), 'human', 'human:test')`, activityID)
		e.WsExec(t, `
			INSERT INTO attachment (id, entity_type, entity_id, filename, byte_size,
				storage_key, checksum, source, captured_by)
			VALUES ($1, 'activity', $2, 'live.pdf', 12, $3, 'sum', 'human', 'human:test')`,
			ids.NewV7(), activityID, key)
	case storedobjects.KindKnowledge:
		corpusID := ids.NewV7()
		e.WsExec(t, `
			INSERT INTO knowledge_corpus
			  (id, name, topic_statement, min_similarity, default_ask, managed_source, captured_by)
			-- managed_source is NULL or 'handbook' by its own CHECK, and this corpus
			-- is neither shipped nor managed.
			VALUES ($1, 'a corpus with a document', 'what it is about', 0.5, false, NULL, 'human:test')`,
			corpusID)
		e.WsExec(t, `
			INSERT INTO knowledge_document (id, corpus_id, filename, content_type, byte_size,
				storage_key, checksum, captured_by)
			VALUES ($1, $2, 'live.pdf', 'application/pdf', 12, $3, 'sum', 'human:test')`,
			ids.NewV7(), corpusID, key)
	case storedobjects.KindLogo:
		// The ICON column, not the wide one: a check reading only logo_object_key
		// calls this unreferenced, which is the four-column finding this pins.
		e.WsExec(t, `UPDATE company SET logo_icon_object_key = $1 WHERE id = $2`,
			key, e.SeedCompany(t, "Referenced Mark GmbH", nil))
	case storedobjects.KindOffer:
		e.WsExec(t, `
			INSERT INTO offer (id, deal_id, offer_number, revision, status, currency,
				pdf_asset_ref, source, captured_by)
			VALUES ($1, $2, 'OF-LIVE-1', 1, 'draft', 'EUR', $3, 'human', 'human:test')`,
			ids.NewV7(), e.SeedWonDealLinkedTo(t), key)
	case storedobjects.KindImport:
		e.WsExec(t, `
			INSERT INTO import_run (connector, status, mapping, source_ref, source, captured_by)
			VALUES ('csv', 'validating', '{}'::jsonb, $1, 'upload', 'human:test')`, key)
	default:
		t.Fatalf("kind %q has no owning row in this test, so the sweep's answer about it is "+
			"unproven: add one when the kind is declared", kind)
	}
}

// Every kind's reference check refuses a seat, and asks nothing for no keys.
//
// The checks answer which objects nothing references, which is the list a reaper
// deletes from — so a seat able to ask it learns which of the installation's files
// are unprotected, and the sweep's own authority is the only one that should.
//
// The empty case is the sweep's ordinary one: it groups the ledger by kind, so a kind
// with nothing provisional reaches its owner with no keys every pass. Answering that
// with a query is a round trip per kind per pass for a question with one answer.
func TestEveryReferenceCheckRefusesASeatAndAnswersNothingForNoKeys(t *testing.T) {
	e := integration.Setup(t)
	owners := unreferencedKeysBy(InstallationDB(e.Pool))
	if len(owners) == 0 {
		t.Fatal("no owners are registered, so this test asserts nothing about any of them")
	}
	seat := e.As(e.Rep1, nil, integration.AdminPerms)
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS),
		"stored_object_reap_worker")

	for kind, check := range owners {
		if _, err := check(seat, []string{"ws/" + string(kind) + "/probe"}); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("the %s check answered a seat with %v, want permission denied: the list of "+
				"unreferenced objects is what a reaper deletes from", kind, err)
		}
		got, err := check(system, nil)
		if err != nil {
			t.Errorf("the %s check with no keys: %v", kind, err)
		}
		if len(got) != 0 {
			t.Errorf("the %s check answered %v for no keys at all", kind, got)
		}
	}
}

// Each writer declares its own kind, and the ledger holds the key under it.
//
// The kind is how the sweep chooses which module to ask, so a writer declaring the
// wrong one sends its key to a table that does not carry it — and the answer that
// comes back is "nothing references this" about a live file. Nothing else in the
// system can catch that: both sides are internally consistent, and a sweep finding
// no orphans is what a healthy installation looks like.
func TestEachWriterDeclaresItsOwnKind(t *testing.T) {
	e := integration.Setup(t)
	db := InstallationDB(e.Pool)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)

	for _, c := range []struct {
		kind   storedobjects.Kind
		record func(string) error
	}{
		{storedobjects.KindLogo, func(base string) error {
			return contacts.NewStore(db).RecordLogoIntent(ctx, base)
		}},
		{storedobjects.KindOffer, func(key string) error {
			return deals.NewStore(db, DealsInstallation()).RecordOfferPdfIntent(ctx, key)
		}},
		{storedobjects.KindImport, func(key string) error {
			// The import declaration is gated on the grant its own upload takes, so
			// the caller here holds it — importerPerms is AdminPerms plus exactly
			// that, which is what the real upload path requires.
			return migration.NewRunStore(db).RecordImportSourceIntent(
				e.As(e.Rep1, nil, importerPerms()), key)
		}},
	} {
		t.Run(string(c.kind), func(t *testing.T) {
			base := "ws/" + string(c.kind) + "/declared-by-its-writer"
			if err := c.record(base); err != nil {
				t.Fatalf("declaring a %s intent: %v", c.kind, err)
			}
			// The logo writer applies its own suffix, so the row is found by PREFIX:
			// what the key ends in is that module's business, and the kind beside it
			// is what the sweep reads.
			var kind string
			if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				return tx.QueryRow(context.Background(), `
					SELECT kind FROM stored_object_intent WHERE storage_key LIKE $1 || '%'`,
					base).Scan(&kind)
			}); err != nil {
				t.Fatalf("reading back the %s declaration: %v", c.kind, err)
			}
			if kind != string(c.kind) {
				t.Errorf("the %s writer declared kind %q, so the sweep asks the wrong table and "+
					"is told nobody references a live file", c.kind, kind)
			}
		})
	}
}

// A reference check that cannot reach the database says so, without saying how.
//
// Two obligations at once. The sweep must not read a failure as "nothing references
// these keys" — that answer deletes every one of them — so the error comes back and
// the slice is nil, because a caller reading keys alongside an error deletes them.
// And the message names the question, never the statement: an error carrying SQL or
// a table name hands a reader the schema.
//
// The failure it reaches is the TRANSACTION's, not a statement's: a cancelled
// context cannot begin one, so nothing inside the callback runs. A swallowed query
// error one layer deeper would need a connection that fails mid-statement, which
// nothing here fakes — what this holds is the layer a lost database actually
// presents.
func TestAReferenceCheckThatCannotReachTheDatabaseSaysSoWithoutTheStatement(t *testing.T) {
	e := integration.Setup(t)
	owners := unreferencedKeysBy(InstallationDB(e.Pool))
	if len(owners) == 0 {
		t.Fatal("no owners are registered, so this test asserts nothing about any of them")
	}
	// Cancelled before the call, so every check meets a connection it cannot use.
	// Nothing about the ledger is mocked: the failure is the real one a lost database
	// produces.
	stopped, cancel := context.WithCancel(principal.SystemActing(
		principal.WithWorkspaceID(context.Background(), e.WS), "stored_object_reap_worker"))
	cancel()

	for kind, check := range owners {
		got, err := check(stopped, []string{"ws/" + string(kind) + "/probe"})
		if err == nil {
			t.Errorf("the %s check answered %v with no database, and an empty answer means "+
				"every key in it is an orphan", kind, got)
			continue
		}
		if got != nil {
			t.Errorf("the %s check answered %v alongside its error: a caller reading the keys "+
				"before the error deletes them", kind, got)
		}
		for _, leak := range []string{"SELECT", "unnest", "NOT EXISTS"} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("the %s check's error carries %q, which hands a reader the statement: %v",
					kind, leak, err)
			}
		}
	}
}
