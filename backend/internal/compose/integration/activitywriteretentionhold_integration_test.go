// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Every activity write that resolved the row storekit.LiveOnly answered 404
// for a row a statutory retention hold had archived, even though
// setActivityAudience declares the 423 the CHECK trigger behind that lock was
// meant to answer. This proves the fix against a genuinely restricted row —
// the same one TestErasureRestrictsAHandelsbriefInsteadOfDestroyingIt proves
// the erasure itself produces — rather than a hand-set restricted_at, which
// would only prove the retry logic and not the row it retries against.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARetentionHeldActivityAnswers423NotNotFoundOnWrite(t *testing.T) {
	e := Setup(t)
	f := seedRestrictionFixture(t, e)
	if err := privacy.NewEraser(e.DB()).ErasePerson(e.Admin(), f.person, "test"); err != nil {
		t.Fatalf("erasing the subject: %v", err)
	}
	held := ids.From[ids.ActivityKind](f.email)
	other := seedBarePerson(t, e)

	var restricted, archived bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT restricted_at IS NOT NULL, archived_at IS NOT NULL FROM activity WHERE id = $1`, f.email).
			Scan(&restricted, &archived)
	}); err != nil {
		t.Fatalf("checking the fixture actually restricted the row: %v", err)
	}
	if !restricted || !archived {
		t.Fatalf("the erasure did not restrict the Handelsbrief (restricted=%v archived=%v); "+
			"every 423 assertion below would prove nothing", restricted, archived)
	}

	assert423 := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: succeeded against a retention-held row, want a refusal", name)
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("%s: answered not-found — the row IS there and held, and this caller is authorized over it", name)
		}
		fault, ok := httperr.Classify(err)
		if !ok || fault.Status != http.StatusLocked {
			t.Fatalf("%s classified as %+v (ok=%v), want 423 locked", name, fault, ok)
		}
	}

	_, err := e.Activities.SetAudience(e.Admin(), held, activities.SetAudienceInput{Audience: "participants"})
	assert423("SetAudience", err)

	subject := "edited"
	_, err = e.Activities.UpdateActivity(e.Admin(), held, activities.UpdateActivityInput{Subject: &subject})
	assert423("UpdateActivity", err)

	_, err = e.Activities.RelinkActivity(e.Admin(), held, activities.RelinkActivityInput{
		EntityType: "person", EntityID: other,
	})
	assert423("RelinkActivity", err)

	_, err = e.Activities.ArchiveActivity(e.Admin(), held, nil)
	assert423("ArchiveActivity", err)

	// A stale If-Version on a held row still owes 423, not 409: every write
	// to a held row is refused by activity_refuse_restricted_mutation
	// regardless of version, and a version-skew answer would invite a
	// refetch-and-retry the row can never accept.
	staleSubject := "edited again"
	_, err = e.Activities.UpdateActivity(e.Admin(), held,
		activities.UpdateActivityInput{Subject: &staleSubject, IfVersion: int64Ptr(999999)})
	assert423("UpdateActivity with a stale If-Version", err)
}

// TestSetAudienceOnAHeldCapturedRowAnswers423NotCapturedAudienceError proves
// the retention refusal outranks the captured-audience one: a row can be
// BOTH held and captured (imported by a mailbox, which is what
// refuseCapturedAudienceWrite normally exists to refuse a direct audience
// write on), and the two refusals answer different things — 423 says
// nothing about this request would ever succeed, 422 says try the owner
// endpoint instead. A held row's request is never fixable that way, so 423
// must win.
func TestSetAudienceOnAHeldCapturedRowAnswers423NotCapturedAudienceError(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)

	heldActivity := SeedIDRow(t, owner, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by,
		                      archived_at, restricted_at, restricted_until, retention_class,
		                      retention_class_at, restricted_reason)
		VALUES ($1, 'email', 'Held and Captured', 'body', now(), 'gmail', 'connector:gmail',
		        now(), now(), now() + interval '5 years', 'commercial_correspondence',
		        now(), 'commercial_correspondence')`)
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, heldActivity, e.AdminUser); err != nil {
		t.Fatalf("seeding the capture_import row: %v", err)
	}
	held := ids.From[ids.ActivityKind](heldActivity)

	_, err := e.Activities.SetAudience(e.Admin(), held, activities.SetAudienceInput{Audience: "participants"})
	if err == nil {
		t.Fatal("SetAudience on a held captured row succeeded, want a refusal")
	}
	var captured *activities.CapturedAudienceError
	if errors.As(err, &captured) {
		t.Fatalf("SetAudience answered CapturedAudienceError (422) for a held row — "+
			"the retention refusal must outrank it, since asking the owner endpoint instead "+
			"could never succeed either: %v", err)
	}
	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != http.StatusLocked {
		t.Fatalf("SetAudience on a held captured row classified as %+v (ok=%v), want 423 locked", fault, ok)
	}
}

// TestARetentionHeldRowStaysHiddenFromAnUnrelatedWriter is the security
// boundary the previous test's fix could have quietly removed: skipping the
// DISCOVER gate for a held row (auth.EnsureActivityWritableIn's live=false
// arm) must not let an unrelated caller learn anything a genuinely missing
// row would not also tell them. The house rule this answers to is
// writescope.go's own: a write-authority answer is a NARROWING of a
// visibility one, never a substitute, and ErrPermissionDenied is owed only
// to a caller the visibility gate already told the row is theirs to read.
// A caller with no relationship to the row — not its author, not linked to
// anything they may write — therefore still answers ErrNotFound, the exact
// answer the skipped DISCOVER gate would have given for a row that is not
// theirs; ErrPermissionDenied here would be a side channel distinguishing
// "held and not mine" from "gone", which the row's own availability clause
// (auth.ActivityAvailableClause) says must not exist for anyone.
// Constructed with a direct INSERT of an already-restricted row rather than
// a live erasure: activity_refuse_restricted_mutation is a BEFORE
// UPDATE/DELETE trigger, so it never fires on this insert, and the only
// thing this test needs is a row that already carries restricted_at and
// archived_at, linked to a person the caller cannot write.
func TestARetentionHeldRowStaysHiddenFromAnUnrelatedWriter(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)

	theirPerson := e.SeedPerson(t, "Held But Unrelated", &e.Rep3)
	e.MakeCapturePrivate(t, "person", theirPerson, e.Rep3)
	heldActivity := SeedIDRow(t, owner, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by,
		                      archived_at, restricted_at, restricted_until, retention_class,
		                      retention_class_at, restricted_reason)
		VALUES ($1, 'email', 'Confidential', 'body', now(), 'manual', 'human:x',
		        now(), now(), now() + interval '5 years', 'commercial_correspondence',
		        now(), 'commercial_correspondence')`)
	LinkActivity(t, owner, heldActivity, "person", theirPerson)
	held := ids.From[ids.ActivityKind](heldActivity)

	// All four write paths, not just one: each composes lockActivityForWrite
	// and auth.EnsureActivityWritableIn independently, so a boundary that
	// held for UpdateActivity alone would still leave the other three able
	// to disclose the row through a 423 an unrelated caller should never see.
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	assertHidden := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s: an unrelated writer on a held row answered %v, want ErrNotFound — "+
				"ErrPermissionDenied (or a 423) here is itself the disclosure that the row exists "+
				"and is held, to a caller the ordinary DISCOVER gate would have told nothing", name, err)
		}
	}

	subject := "rewritten"
	_, err := e.Activities.UpdateActivity(rep, held, activities.UpdateActivityInput{Subject: &subject})
	assertHidden("UpdateActivity", err)

	_, err = e.Activities.SetAudience(rep, held, activities.SetAudienceInput{Audience: "workspace"})
	assertHidden("SetAudience", err)

	_, err = e.Activities.RelinkActivity(rep, held, activities.RelinkActivityInput{
		EntityType: "person", EntityID: theirPerson,
	})
	assertHidden("RelinkActivity", err)

	_, err = e.Activities.ArchiveActivity(rep, held, nil)
	assertHidden("ArchiveActivity", err)
}

// TestAnUnboundedCallerStillFacesTheAudienceArmOnAHeldRow is the second half
// of the security boundary the previous test checks: OWNERSHIP is not a
// substitute for AUDIENCE, and an unbounded human is not a substitute for
// the system principal. auth.ActivityContentClause composes BOTH the
// discover clause and auth.ActivityAudienceArm for every non-system
// caller — EnsureActivityWritableIn's live=false arm skips only the
// liveness half of that gate; it must not silently skip the audience half
// too. The fixture is a task assigned directly to the caller (satisfying
// EnsureActivityWritableIn's OWNERSHIP arm, assignee_id) but narrowed to
// `participants` with the caller on none of the audience's own arms
// (captured_by, capture_import, activity_participant, or a selected
// membership row) — ownership and audience answer different questions, and
// this row is built to answer them differently. The caller is e.Admin(),
// an UNBOUNDED principal: ActivityAudienceArm exempts only the system
// principal, so an unbounded human being waved past the ownership check
// (auth.Unbounded's own short-circuit) must not also be waved past this.
func TestAnUnboundedCallerStillFacesTheAudienceArmOnAHeldRow(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)

	heldTask := SeedIDRow(t, owner, `
		INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by,
		                      assignee_id, audience,
		                      archived_at, restricted_at, restricted_until, retention_class,
		                      retention_class_at, restricted_reason)
		VALUES ($1, 'task', 'Confidential Task', 'body', now(), 'manual', 'human:someone-else',
		        $2, 'participants',
		        now(), now(), now() + interval '5 years', 'commercial_correspondence',
		        now(), 'commercial_correspondence')`, e.AdminUser)
	held := ids.From[ids.ActivityKind](heldTask)

	subject := "rewritten"
	if _, err := e.Activities.UpdateActivity(e.Admin(), held, activities.UpdateActivityInput{Subject: &subject}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an unbounded caller outside a held row's narrowed audience answered %v, want ErrNotFound — "+
			"the row's assignee_id passing the OWNERSHIP arm is not a licence to skip the AUDIENCE arm too, "+
			"and an unbounded human is bound by that arm exactly like anyone but the system principal", err)
	}
}

// seedBarePerson is a relink target unrelated to the restriction fixture: the
// held activity must gain a genuinely NEW link (not a duplicate the ON
// CONFLICT DO NOTHING no-ops on) for the relink to reach the row UPDATE its
// CHECK trigger refuses.
func seedBarePerson(t *testing.T, e *Env) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`INSERT INTO person (id, full_name, first_name, source, captured_by)
			 VALUES ($1, 'Relink Target', 'Relink', 'manual', 'human:x')`, id)
		return err
	}); err != nil {
		t.Fatalf("seeding a relink target person: %v", err)
	}
	return id
}
