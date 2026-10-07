// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The cached relationship brief is generated prose about the subject, held per
// reader. Both privacy acts leave the contact row standing, so the ON DELETE
// CASCADE on contact_id never fires and nothing but a named purge reaches it.
//
// Both acts are asserted because they are separate statements in separate
// files, and an operator told a contact was ANONYMIZED has been told the same
// thing about findability as one told they were erased.

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/contactbrief"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// briefUpsertSQL mirrors contactbrief.Service.save. A mirror is a liability, so
// TestTheBriefCacheTestsMirrorTheWriterTheyStandIn holds the two together.
const briefUpsertSQL = `
	WITH live AS (
	    SELECT id FROM contact WHERE id = $2 AND archived_at IS NULL FOR SHARE
	)
	INSERT INTO contact_brief (user_id, contact_id, fingerprint,
	                          generated_at, generated_by, payload)
	SELECT $1, live.id, $3, $4, $5, $6 FROM live
	ON CONFLICT (user_id, contact_id) DO UPDATE
	SET fingerprint = EXCLUDED.fingerprint,
	    generated_at = EXCLUDED.generated_at,
	    generated_by = EXCLUDED.generated_by,
	    payload = EXCLUDED.payload`

func TestErasureDestroysTheCachedBriefEveryReaderHeld(t *testing.T) {
	e := integration.Setup(t)
	subject := e.SeedContact(t, "Briefed Subject", &e.AdminUser)
	bystander := e.SeedContact(t, "Unrelated Contact", &e.AdminUser)
	cacheABrief(t, e, subject, "fp-1")
	cacheABrief(t, e, bystander, "fp-1")

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), subject, "subject request"); err != nil {
		t.Fatalf("EraseContact → %v", err)
	}
	if n := cachedBriefs(t, e, subject); n != 0 {
		t.Errorf("%d cached brief(s) survive the erasure, each holding what the model was told "+
			"about this subject", n)
	}
	if n := cachedBriefs(t, e, bystander); n != 1 {
		t.Errorf("the bystander's cached brief count is %d, want 1 — the purge is keyed on one "+
			"contact, and a statement that wiped the installation's caches would read as a pass "+
			"on the assertion above", n)
	}
}

func TestAnonymisingAContactDestroysTheCachedBriefToo(t *testing.T) {
	e := integration.Setup(t)
	subject := e.SeedContact(t, "Briefed Subject", &e.AdminUser)
	bystander := e.SeedContact(t, "Unrelated Contact", &e.AdminUser)
	cacheABrief(t, e, subject, "fp-1")
	cacheABrief(t, e, bystander, "fp-1")

	service := NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := service.AnonymiseContacts(e.Admin(), []ids.UUID{subject}, privacy.PurgeOwnerRule); err != nil {
		t.Fatalf("AnonymiseContacts → %v", err)
	}
	if n := cachedBriefs(t, e, subject); n != 0 {
		t.Errorf("%d cached brief(s) survive the anonymise, still answering with the name it cleared", n)
	}
	if n := cachedBriefs(t, e, bystander); n != 1 {
		t.Errorf("the bystander's cached brief count is %d, want 1", n)
	}
}

// cacheABrief writes one reader's cached brief for the subject.
//
// The row goes in directly rather than through contactbrief.Service, which
// would need a stubbed model lane and an assembled contact page to produce a
// payload this assertion never reads: the purge is keyed on contact_id alone.
// The shape is the catalog's — payload NOT NULL, generated_by one of two
// values — and cachedBriefs is read once BEFORE each act, so a column this
// fixture stops matching fails the insert rather than quietly testing nothing.
func cacheABrief(t *testing.T, e *integration.Env, subject ids.UUID, fingerprint string) {
	t.Helper()
	if _, err := e.Pool.Exec(e.Admin(), `
		INSERT INTO contact_brief (user_id, contact_id, fingerprint, generated_by, payload)
		VALUES ($1, $2, $4, 'deterministic', $3::jsonb)`,
		e.AdminUser, subject,
		`{"headline":"Briefed Subject asked about the retrofit timeline"}`, fingerprint); err != nil {
		t.Fatalf("caching a brief for the subject: %v", err)
	}
	if n := cachedBriefs(t, e, subject); n != 1 {
		t.Fatalf("the fixture cached %d briefs, want 1 — the assertions below would prove nothing", n)
	}
}

func cachedBriefs(t *testing.T, e *integration.Env, subject ids.UUID) int {
	t.Helper()
	var n int
	if err := e.Pool.QueryRow(e.Admin(),
		`SELECT count(*) FROM contact_brief WHERE contact_id = $1`, subject).Scan(&n); err != nil {
		t.Fatalf("counting the subject's cached briefs: %v", err)
	}
	return n
}

// A brief composed before an erasure must not land after it. Generation reads
// the record, waits on a model and then saves, so the window is wide enough for
// the whole erasure to run inside it.
//
// Both halves of the guard are asserted, because the cheap one looks sufficient
// and is not: the archived_at test alone reads the pre-erasure row under MVCC
// and writes anyway, which is why the statement takes the row.
func TestABriefComposedBeforeAnErasureCannotLandAfterIt(t *testing.T) {
	e := integration.Setup(t)
	subject := e.SeedContact(t, "Briefed Subject", &e.AdminUser)

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), subject, "subject request"); err != nil {
		t.Fatalf("EraseContact → %v", err)
	}
	if err := saveABrief(e, subject); err != nil {
		t.Fatalf("the late save errored rather than writing nothing: %v", err)
	}
	if n := cachedBriefs(t, e, subject); n != 0 {
		t.Errorf("a brief landed for an erased subject: %d row(s), holding the prose the erasure "+
			"destroyed", n)
	}

	// And the interleave the archived_at test cannot see: the erasure is in
	// flight and uncommitted, so the row still reads as live. The save must WAIT
	// on it rather than write, which a lock timeout proves without depending on
	// how long anything takes to run.
	live := e.SeedContact(t, "Still Live", &e.AdminUser)
	erasing, err := e.Pool.Begin(e.Admin())
	if err != nil {
		t.Fatalf("opening the erasing transaction: %v", err)
	}
	defer func() {
		if err := erasing.Rollback(e.Admin()); err != nil {
			t.Errorf("rolling back the erasing transaction: %v", err)
		}
	}()
	if _, err := erasing.Exec(e.Admin(),
		`UPDATE contact SET archived_at = now() WHERE id = $1`, live); err != nil {
		t.Fatalf("archiving the contact in the open transaction: %v", err)
	}
	if err := saveABriefWaitingAtMost(t, e, live, "250ms"); err == nil {
		t.Error("the save completed while an erasure held the contact row, so it never took the " +
			"row and an in-flight erasure cannot stop it")
	} else if !strings.Contains(err.Error(), "lock timeout") && !strings.Contains(err.Error(), "55P03") {
		t.Errorf("the save failed for a reason other than waiting on the row: %v", err)
	}
}

// saveABrief runs the cache write the brief service runs, as its own statement
// rather than through Service.save: reaching that unexported path needs an
// assembled page and a model lane, and what is under test is the statement.
func saveABrief(e *integration.Env, contact ids.UUID) error {
	_, err := e.Pool.Exec(e.Admin(), briefUpsertSQL,
		e.AdminUser, contact, "fp-late", time.Now(), "deterministic",
		`{"headline":"composed before the erasure"}`)
	return err
}

func saveABriefWaitingAtMost(t *testing.T, e *integration.Env, contact ids.UUID, wait string) error {
	t.Helper()
	tx, err := e.Pool.Begin(e.Admin())
	if err != nil {
		return err
	}
	defer func() {
		if err := tx.Rollback(e.Admin()); err != nil {
			t.Errorf("rolling back the racing save: %v", err)
		}
	}()
	if _, err := tx.Exec(e.Admin(), `SET LOCAL lock_timeout = '`+wait+`'`); err != nil {
		return err
	}
	_, err = tx.Exec(e.Admin(), briefUpsertSQL,
		e.AdminUser, contact, "fp-racing", time.Now(), "deterministic",
		`{"headline":"composed while the erasure ran"}`)
	return err
}

// The real writer refuses after an erasure, driven through contactbrief.Service
// rather than the statement: what the service runs is what production runs, and
// a guard proven only about the test's own copy of the SQL proves nothing about
// the writer.
//
// The model lane is nil, which Write answers with its deterministic floor — so
// this needs no stubbed model, only the page the brief is written from.
func TestTheBriefWriterItselfDeclinesToCacheAnErasedSubject(t *testing.T) {
	e := integration.Setup(t)
	subject := e.SeedContact(t, "Briefed Subject", &e.AdminUser)
	svc := contactbrief.NewService(e.Pool, briefPageOf(subject), nil, "test-routing", time.Now)

	if _, err := svc.Get(e.Admin(), ids.From[ids.ContactKind](subject), false); err != nil {
		t.Fatalf("writing the brief of a live contact: %v", err)
	}
	if n := cachedBriefs(t, e, subject); n != 1 {
		t.Fatalf("the writer cached %d briefs for a live contact, want 1 — the assertion below "+
			"would pass on a writer that never writes", n)
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), subject, "subject request"); err != nil {
		t.Fatalf("EraseContact → %v", err)
	}
	if _, err := svc.Get(e.Admin(), ids.From[ids.ContactKind](subject), true); err != nil {
		t.Fatalf("the writer errored for an erased subject rather than writing nothing: %v", err)
	}
	if n := cachedBriefs(t, e, subject); n != 0 {
		t.Errorf("the writer cached %d brief(s) for an erased subject", n)
	}
}

// briefPageOf assembles the contact page the brief is written from, which is
// the service's one injected collaborator.
func briefPageOf(contact ids.UUID) contactbrief.Assembler {
	return assembleFunc(func(ctx context.Context, id ids.ContactID) (crmcontracts.Contact360, error) {
		return crmcontracts.Contact360{
			Contact: crmcontracts.Contact{Id: openapitypes.UUID(contact), FullName: "Briefed Subject"},
		}, nil
	})
}

type assembleFunc func(context.Context, ids.ContactID) (crmcontracts.Contact360, error)

func (f assembleFunc) Assemble(ctx context.Context, id ids.ContactID) (crmcontracts.Contact360, error) {
	return f(ctx, id)
}

// A cache the erasure cannot destroy stops the erasure.
//
// The purge is one statement inside the Art. 17 transaction, and the whole point
// of it being there is that a half-erasure never commits: the subject stays
// named if their cached brief could not be destroyed, rather than being reported
// erased while a model's prose about them survives.
//
// Both acts are driven, because each propagates the failure through its own
// step — the derived rows on one side, the judgements on the other.
func TestAnUndestroyableCachedBriefStopsTheWholeErasure(t *testing.T) {
	e := integration.Setup(t)
	refuseBriefDeletes(t)
	subject := e.SeedContact(t, "Briefed Subject", &e.AdminUser)
	cacheABrief(t, e, subject, refusedFingerprint)

	err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), subject, "subject request")
	if err == nil {
		t.Fatal("the erasure reported success though the cached brief could not be destroyed")
	}
	if !strings.Contains(err.Error(), "cached relationship brief") {
		t.Errorf("the failure does not say which step could not finish: %v", err)
	}
	if name := contactName(t, e, subject); name != "Briefed Subject" {
		t.Errorf("the contact reads %q, so the erasure committed a half: the name was destroyed "+
			"inside a transaction that could not finish its own cache purge", name)
	}

	service := NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := service.AnonymiseContacts(e.Admin(), []ids.UUID{subject}, privacy.PurgeOwnerRule); err == nil {
		t.Error("the anonymise reported success though the cached brief could not be destroyed")
	}
	if name := contactName(t, e, subject); name != "Briefed Subject" {
		t.Errorf("the contact reads %q after the anonymise, so that act committed a half too", name)
	}
}

// refusedFingerprint marks the one cached brief the trigger below refuses to
// let go. No other fixture writes it.
const refusedFingerprint = "refuse-this-delete"

// refuseBriefDeletes makes the DELETE of one cached brief fail, which is how a
// statement inside somebody else's transaction can be made to fail without
// touching the rows the test is about.
//
// Cleanup drops the trigger AND its function, and runs on a failing test too.
// What cleanup cannot cover is the process dying between the CREATE and it — a
// timeout, an interrupt, an OOM — and the serial lane's database persists, so a
// trigger left behind would be inherited. Hence the row condition: it fires only
// for a fingerprint this test writes, which the next Setup's data reset clears,
// so what survives a crash fires for nothing.
func refuseBriefDeletes(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatalf("connecting as the schema owner: %v", err)
	}
	t.Cleanup(func() {
		for _, stmt := range []string{
			`DROP TRIGGER IF EXISTS trg_refuse_brief_deletes ON contact_brief`,
			`DROP FUNCTION IF EXISTS refuse_brief_deletes()`,
		} {
			if _, err := owner.Exec(context.Background(), stmt); err != nil {
				t.Errorf("dropping the refusing trigger: %v", err)
			}
		}
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
	})
	for _, stmt := range []string{
		`CREATE OR REPLACE FUNCTION refuse_brief_deletes() RETURNS trigger AS $$
		 BEGIN RAISE EXCEPTION 'the cache refuses deletion'; END $$ LANGUAGE plpgsql`,
		`CREATE TRIGGER trg_refuse_brief_deletes BEFORE DELETE ON contact_brief
		 FOR EACH ROW WHEN (OLD.fingerprint = '` + refusedFingerprint + `')
		 EXECUTE FUNCTION refuse_brief_deletes()`,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("installing the refusing trigger: %v", err)
		}
	}
}

func contactName(t *testing.T, e *integration.Env, contact ids.UUID) string {
	t.Helper()
	var name string
	if err := e.Pool.QueryRow(e.Admin(),
		`SELECT full_name FROM contact WHERE id = $1`, contact).Scan(&name); err != nil {
		t.Fatalf("reading the contact's name: %v", err)
	}
	return name
}
