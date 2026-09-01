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

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	subject := "rewritten"
	if _, err := e.Activities.UpdateActivity(rep, held, activities.UpdateActivityInput{Subject: &subject}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an unrelated writer on a held row answered %v, want ErrNotFound — "+
			"ErrPermissionDenied here is itself the disclosure that the row exists and is held, "+
			"to a caller the ordinary DISCOVER gate would have told nothing", err)
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
