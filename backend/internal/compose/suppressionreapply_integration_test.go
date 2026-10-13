// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Erased stays erased, whatever brought it back.

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const resurrectedAddress = "geloescht@betroffene.example"

// The case the restore drill exists to catch: a subject was erased, a restore
// put them back, and the record now looks exactly like one that was never
// erased. The list is the standing instruction, so the pass takes them again.
func TestASubjectBroughtBackAfterErasureIsErasedAgain(t *testing.T) {
	e := integration.Setup(t)
	eraser := privacy.NewEraser(InstallationDB(e.Pool))

	// The erasure happened, and its suppression outlived the row — which is
	// what a restore to a point AFTER the erasure leaves behind.
	seedSuppressedEmail(t, resurrectedAddress)
	// The restore then puts the subject back.
	resurrected := seedContactWithEmail(t, e, "Die Betroffene", resurrectedAddress)

	took, err := eraser.ReapplySuppressions(e.Admin())
	if err != nil {
		t.Fatalf("ReapplySuppressions: %v", err)
	}
	if took != 1 {
		t.Fatalf("re-erased %d, want the resurrected subject taken again", took)
	}
	if live := liveEmails(t, e, resurrected); live != 0 {
		t.Fatalf("the resurrected subject still holds %d address(es)", live)
	}
}

// Zero is the answer on a healthy installation, and it is evidence rather
// than a no-op: a pass that erased something here would be destroying data
// nobody asked it to.
func TestReapplyingSuppressionsTakesNothingFromAHealthyInstallation(t *testing.T) {
	e := integration.Setup(t)
	untouched := seedContactWithEmail(t, e, "Die Kundin", "kundin@partner.example")

	took, err := privacy.NewEraser(InstallationDB(e.Pool)).ReapplySuppressions(e.Admin())
	if err != nil {
		t.Fatalf("ReapplySuppressions: %v", err)
	}
	if took != 0 {
		t.Fatalf("re-erased %d on an installation with nothing suppressed", took)
	}
	if live := liveEmails(t, e, untouched); live != 1 {
		t.Fatalf("a contact nobody suppressed lost their address: %d left", live)
	}
}

// A suppression naming somebody the installation does not hold is not a
// finding. The list is installation-wide and outlives the rows it was written
// about, so most entries match nothing for good.
func TestASuppressionWithNoLiveSubjectIsNotAFinding(t *testing.T) {
	e := integration.Setup(t)
	seedSuppressedEmail(t, "niemand@nirgends.example")

	took, err := privacy.NewEraser(InstallationDB(e.Pool)).ReapplySuppressions(e.Admin())
	if err != nil {
		t.Fatalf("ReapplySuppressions: %v", err)
	}
	if took != 0 {
		t.Fatalf("re-erased %d for a suppression matching nobody", took)
	}
}

// The match follows the eraser's own normalization rather than the column's.
//
// contact_email enforces `email = lower(email)` and nothing about surrounding
// space, so an import CAN store "  geloescht@betroffene.example  " — and a
// subject who came back with a stray space would otherwise read as somebody
// the list had never heard of. What makes the two spellings one subject is
// SuppressionHash, the same function the eraser wrote the entry with.
//
// A hand-written lower(btrim()) in SQL would match this particular pair too.
// It is not used because it would be a SECOND spelling of the hash, which is
// the mistake suppression.go names in its own header, and because Go and
// Postgres do not lowercase every alphabet identically — the case where they
// part is not this test, and that is exactly why the rule lives in one place
// rather than being reimplemented wherever it is convenient.
func TestTheMatchFollowsTheErasersOwnNormalization(t *testing.T) {
	e := integration.Setup(t)
	// The suppression was written from one spelling...
	seedSuppressedEmail(t, "  Geloescht@Betroffene.example  ")
	// ...and the resurrection arrived in another the column permits.
	resurrected := seedContactWithEmail(t, e, "Die Betroffene", "  geloescht@betroffene.example  ")

	took, err := privacy.NewEraser(InstallationDB(e.Pool)).ReapplySuppressions(e.Admin())
	if err != nil {
		t.Fatalf("ReapplySuppressions: %v", err)
	}
	if took != 1 {
		t.Fatalf("re-erased %d, want the same subject recognized through its spelling", took)
	}
	if live := liveEmails(t, e, resurrected); live != 0 {
		t.Fatalf("a differently-spelled resurrection survived: %d address(es) left", live)
	}
}

func seedSuppressedEmail(t *testing.T, email string) {
	t.Helper()
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO erasure_suppression (kind, value_hash) VALUES ('email', $1)
		ON CONFLICT DO NOTHING`, storekit.SuppressionHash(email)); err != nil {
		t.Fatalf("seeding the suppression: %v", err)
	}
}

func seedContactWithEmail(t *testing.T, e *integration.Env, name, email string) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, name, nil)
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO contact_email (id, contact_id, email, email_type, is_primary, source, captured_by)
		VALUES ($1, $2, $3, 'work', true, 'manual', 'human:restore')`,
		ids.NewV7(), contact, email); err != nil {
		t.Fatalf("seeding the address: %v", err)
	}
	return contact
}

func liveEmails(t *testing.T, e *integration.Env, contact ids.UUID) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM contact_email WHERE contact_id = $1`, contact).Scan(&n)
	}); err != nil {
		t.Fatalf("counting addresses: %v", err)
	}
	return n
}

// The pass reads every contact in the installation to decide which ones the
// list already ended. A caller who may not delete a contact has no business
// being handed that reading either, so the gate is asked before the walk
// begins rather than per subject at the end of it.
func TestReapplyingSuppressionsRefusesACallerWhoMayNotErase(t *testing.T) {
	e := integration.Setup(t)
	seedSuppressedEmail(t, resurrectedAddress)
	resurrected := seedContactWithEmail(t, e, "Die Betroffene", resurrectedAddress)

	_, err := privacy.NewEraser(InstallationDB(e.Pool)).
		ReapplySuppressions(e.As(e.Rep1, nil, integration.AccountRepPerms))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("err = %v, want the contact gate to refuse a seat that may not erase", err)
	}
	// And nothing was taken on the way to being refused.
	if live := liveEmails(t, e, resurrected); live != 1 {
		t.Fatalf("a refused pass still erased something: %d address(es) left", live)
	}
}

// A legal hold is somebody else's obligation over ONE record. It must not
// become the reason the whole installation keeps data it was told to erase:
// the pass takes everyone it may and reports the one it could not.
func TestAHeldSubjectDoesNotStopTheOthersBeingTakenAgain(t *testing.T) {
	e := integration.Setup(t)
	seedSuppressedEmail(t, resurrectedAddress)
	seedSuppressedEmail(t, "zweite@betroffene.example")

	held := seedContactWithEmail(t, e, "Unter Verfügung", resurrectedAddress)
	takeable := seedContactWithEmail(t, e, "Die Zweite", "zweite@betroffene.example")
	placeLegalHold(t, held)

	took, err := privacy.NewEraser(InstallationDB(e.Pool)).ReapplySuppressions(e.Admin())
	// The refusal travels back rather than being swallowed: the count says
	// what was done and the error says what could not be.
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("err = %v, want the hold reported", err)
	}
	if took != 1 {
		t.Fatalf("re-erased %d, want the subject that could be taken", took)
	}
	if live := liveEmails(t, e, takeable); live != 0 {
		t.Error("a takeable resurrection was left standing behind a hold on somebody else")
	}
	if live := liveEmails(t, e, held); live != 1 {
		t.Error("a contact under legal hold was erased anyway")
	}
}

func placeLegalHold(t *testing.T, contact ids.UUID) {
	t.Helper()
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(),
		`UPDATE contact SET legal_hold = true WHERE id = $1`, contact); err != nil {
		t.Fatalf("placing the legal hold: %v", err)
	}
}

// The restore this whole path exists for: the database went back to a point
// BEFORE the erasure, so the subject's rows came back AND the record that they
// were erased went away with them. The database's own list is empty and knows
// nothing. The exported journal is what answers.
func TestAJournalledSuppressionSurvivesTheListRollingBack(t *testing.T) {
	e := integration.Setup(t)
	store := blobstore.NewMemory()
	eraser := privacy.NewEraser(InstallationDB(e.Pool)).WithBlobstore(store)

	// Before the backup: the subject was erased and the list exported.
	seedSuppressedEmail(t, resurrectedAddress)
	exported, err := eraser.ExportSuppressions(e.Admin())
	if err != nil {
		t.Fatalf("ExportSuppressions: %v", err)
	}
	if exported != 1 {
		t.Fatalf("exported %d entries, want the suppression carried out of the database", exported)
	}

	// The restore: the rows are back and the list went with the rollback.
	clearSuppressionList(t)
	resurrected := seedContactWithEmail(t, e, "Die Betroffene", resurrectedAddress)

	took, err := eraser.ReapplySuppressions(e.Admin())
	if err != nil {
		t.Fatalf("ReapplySuppressions: %v", err)
	}
	if took != 1 {
		t.Fatalf("re-erased %d, want the journal to have caught what the list forgot", took)
	}
	if live := liveEmails(t, e, resurrected); live != 0 {
		t.Fatalf("the subject survived a restore their erasure should have outlived: %d address(es)", live)
	}
}

// The export carries the fingerprint and nothing else. An export that wrote
// the address would reconstitute exactly what the erasure destroyed, and put
// it somewhere the erasure does not reach.
func TestTheExportedListNeverCarriesTheIdentifierItself(t *testing.T) {
	e := integration.Setup(t)
	store := blobstore.NewMemory()
	eraser := privacy.NewEraser(InstallationDB(e.Pool)).WithBlobstore(store)
	seedSuppressedEmail(t, resurrectedAddress)

	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("ExportSuppressions: %v", err)
	}

	reader, _, err := store.Get(e.Admin(),
		"erasure-suppression/email/"+storekit.SuppressionHash(resurrectedAddress))
	if err != nil {
		t.Fatalf("reading the exported entry: %v", err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Errorf("closing the exported entry: %v", err)
		}
	}()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading the entry body: %v", err)
	}
	if strings.Contains(string(body), resurrectedAddress) {
		t.Fatalf("the exported entry carries the address it was written to forget: %q", body)
	}
}

// Running it twice is not two lists. The export is a sync, so it can be run
// before every backup without anybody working out what changed.
func TestExportingTheListTwiceIsIdempotent(t *testing.T) {
	e := integration.Setup(t)
	store := blobstore.NewMemory()
	eraser := privacy.NewEraser(InstallationDB(e.Pool)).WithBlobstore(store)
	seedSuppressedEmail(t, resurrectedAddress)

	first, err := eraser.ExportSuppressions(e.Admin())
	if err != nil {
		t.Fatalf("first export: %v", err)
	}
	second, err := eraser.ExportSuppressions(e.Admin())
	if err != nil {
		t.Fatalf("second export: %v", err)
	}
	if first != second || first != 1 {
		t.Fatalf("exports reported %d then %d, want the same one entry both times", first, second)
	}
}

// A deployment with no object store is told so rather than answering an
// export that did not happen: the value of this is that somebody can rely on
// it having run before a backup.
func TestExportingWithoutAnObjectStoreSaysSo(t *testing.T) {
	e := integration.Setup(t)
	if _, err := privacy.NewEraser(InstallationDB(e.Pool)).ExportSuppressions(e.Admin()); err == nil {
		t.Fatal("an export with nowhere to write reported success")
	}
}

func clearSuppressionList(t *testing.T) {
	t.Helper()
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `DELETE FROM erasure_suppression`); err != nil {
		t.Fatalf("rolling the suppression list back: %v", err)
	}
}

// failingBlobs answers every question with an outage. Not ErrNotFound — the
// difference between "no such object" and "could not ask" is the whole point
// of the two tests below.
type failingBlobs struct{ err error }

func (f failingBlobs) Put(context.Context, string, io.Reader, int64, string) error { return f.err }

func (f failingBlobs) Get(context.Context, string) (io.ReadCloser, blobstore.Object, error) {
	return nil, blobstore.Object{}, f.err
}
func (f failingBlobs) Delete(context.Context, string) error              { return f.err }
func (f failingBlobs) DeletePrefix(context.Context, string) (int, error) { return 0, f.err }
func (f failingBlobs) Health(context.Context) error                      { return f.err }

// A store that cannot answer is not a "no".
//
// Reporting one would let an outage read as "this subject was never erased",
// which is the exact failure this whole path exists to prevent — and it would
// do it silently, leaving a resurrected subject standing and the pass
// reporting a clean run.
func TestAnUnreachableJournalIsNotAnAnswerThatNobodyWasErased(t *testing.T) {
	e := integration.Setup(t)
	outage := errors.New("the object store is unreachable")
	eraser := privacy.NewEraser(InstallationDB(e.Pool)).WithBlobstore(failingBlobs{err: outage})

	// A live subject the database's own list does not name, so the pass has to
	// ask the journal about them — and the journal cannot answer.
	resurrected := seedContactWithEmail(t, e, "Die Betroffene", resurrectedAddress)

	_, err := eraser.ReapplySuppressions(e.Admin())
	if err == nil {
		t.Fatal("an unreachable journal reported a clean run")
	}
	// And nothing was destroyed on the strength of an answer nobody gave.
	if live := liveEmails(t, e, resurrected); live != 1 {
		t.Fatalf("a subject was erased while the list could not be read: %d address(es)", live)
	}
}

// An export that could not write is not an export. Saying it succeeded would
// leave somebody relying on a list that is not there, which is worse than
// having no export at all — they would stop checking.
func TestAnExportThatCannotWriteSaysSo(t *testing.T) {
	e := integration.Setup(t)
	seedSuppressedEmail(t, resurrectedAddress)
	eraser := privacy.NewEraser(InstallationDB(e.Pool)).
		WithBlobstore(failingBlobs{err: errors.New("the object store is full")})

	if _, err := eraser.ExportSuppressions(e.Admin()); err == nil {
		t.Fatal("an export that wrote nothing reported success")
	}
}

// A subject with two suppressed addresses is one subject. Erasing them twice
// would write a second tombstone for a record the first one already ended.
func TestASubjectWithTwoSuppressedAddressesIsTakenOnce(t *testing.T) {
	e := integration.Setup(t)
	store := blobstore.NewMemory()
	eraser := privacy.NewEraser(InstallationDB(e.Pool)).WithBlobstore(store)

	const second = "zweite@betroffene.example"
	seedSuppressedEmail(t, resurrectedAddress)
	seedSuppressedEmail(t, second)
	if _, err := eraser.ExportSuppressions(e.Admin()); err != nil {
		t.Fatalf("ExportSuppressions: %v", err)
	}
	clearSuppressionList(t)

	// Both addresses come back on the ONE contact, and only the journal knows
	// either of them.
	resurrected := seedContactWithEmail(t, e, "Die Betroffene", resurrectedAddress)
	addEmail(t, resurrected, second)

	took, err := eraser.ReapplySuppressions(e.Admin())
	if err != nil {
		t.Fatalf("ReapplySuppressions: %v", err)
	}
	if took != 1 {
		t.Fatalf("re-erased %d, want the subject counted once for two addresses", took)
	}
	if live := liveEmails(t, e, resurrected); live != 0 {
		t.Fatalf("addresses survived: %d", live)
	}
}

func addEmail(t *testing.T, contact ids.UUID, email string) {
	t.Helper()
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO contact_email (id, contact_id, email, email_type, is_primary, source, captured_by)
		VALUES ($1, $2, $3, 'work', false, 'manual', 'human:restore')`,
		ids.NewV7(), contact, email); err != nil {
		t.Fatalf("seeding the second address: %v", err)
	}
}

// The export job writes the journal, which is what a restore replays from.
//
// ExportSuppressions asks to be run on a schedule and nothing ran it, so the
// entries a restore would need were never written. A restore to a point before
// an erasure then brought the subject back with nothing left to refuse them.
func TestTheSuppressionJournalJobWritesWhatARestoreWouldLose(t *testing.T) {
	e := integration.Setup(t)
	store := &countingBlobstore{Store: blobstore.NewMemory()}
	seedSuppressedEmail(t, "erased@journal.example")

	worker := &suppressionJournalWorker{pool: e.Pool, blob: store, log: slog.New(slog.DiscardHandler)}
	if err := worker.exportWorkspace(e.Admin(), e.WS); err != nil {
		t.Fatalf("exporting the journal: %v", err)
	}

	if len(store.puts) == 0 {
		t.Fatal("the export wrote nothing, so a restore has no journal to replay")
	}
	// Neither the key nor the body carries the address. The journal keeps only
	// a fingerprint, because the value is gone.
	//
	// An entry carrying it would reconstitute what the erasure destroyed.
	for key, body := range store.puts {
		if strings.Contains(key, "erased@journal.example") || strings.Contains(body, "erased@journal.example") {
			t.Errorf("the journal entry %s carries the erased address", key)
		}
	}
}

// countingBlobstore records what the export wrote. The object store is a true
// boundary, and the memory store it wraps offers no enumeration.
type countingBlobstore struct {
	blobstore.Store
	puts map[string]string
}

func (c *countingBlobstore) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if c.puts == nil {
		c.puts = map[string]string{}
	}
	c.puts[key] = string(body)
	return c.Store.Put(ctx, key, strings.NewReader(string(body)), size, contentType)
}
