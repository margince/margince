// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The decay lane's two reads against a real database.
//
// Both claims are about the person row scope, which is SQL: the candidate read
// scopes at source so the cap is spent on rows the reader may see, and the
// batched derivation admits only readable contacts. A unit test with
// hand-built rows cannot fail on either — nor on a column name — so the
// refusal AND the admission are both asserted here.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// seedLapsedPair seeds one exchange old enough to be past the §4 quiet
// threshold, then folds it through the REAL projection writer.
//
// The edge row is never inserted directly. A hand-written row is a row the
// fold never produced, so a test seeded that way can pass over a projection
// that no longer writes what the read expects — which is the whole failure
// this lane's candidate half would hide.
func seedLapsedPair(t *testing.T, e *Env, colleague, person ids.UUID, subject string) {
	t.Helper()
	seedLapsedPairAt(t, e, colleague, person, subject, relstrength.QuietDays+35)
}

// seedLapsedPairAt is the same, with the silence's AGE named — for the cases
// that turn on which contact is oldest rather than only on being past the
// threshold.
func seedLapsedPairAt(t *testing.T, e *Env, colleague, person ids.UUID, subject string, daysAgo int) {
	t.Helper()
	owner := OwnerConn(t)
	ctx := context.Background()
	quietSince := time.Now().UTC().AddDate(0, 0, -daysAgo)

	activity := ids.NewV7()
	if _, err := owner.Exec(ctx, `
		INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
		VALUES ($1, 'email', $2, $3, 'outbound', 'manual', 'human:x')`,
		activity, subject, quietSince); err != nil {
		t.Fatal(err)
	}
	LinkActivity(t, owner, activity, "person", person)
	for _, seed := range []struct {
		column string
		id     ids.UUID
		role   string
	}{{"user_id", colleague, "from"}, {"person_id", person, "to"}} {
		if _, err := owner.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, `+seed.column+`, role) VALUES ($1, $2, $3)`,
			activity, seed.id, seed.role); err != nil {
			t.Fatal(err)
		}
	}
	wsCtx := principal.WithWorkspaceID(ctx, e.WS)
	if err := database.WithWorkspaceTx(wsCtx, e.Pool, func(tx pgx.Tx) error {
		return search.RecomputeEdgesForActivities(wsCtx, tx, []ids.UUID{activity})
	}); err != nil {
		t.Fatalf("folding the seeded exchange into the projection: %v", err)
	}
}

// A quiet edge to a contact outside the caller's row scope is not a candidate.
// The refusal alone cannot show the read works — a query that returned nothing
// for everyone would pass it — so the same call must admit the caller's own
// lapsed contact.
func TestQuietCandidatesCarryOnlyContactsInsideRowScope(t *testing.T) {
	e := Setup(t)
	mine := e.SeedPerson(t, "Anna Weber", &e.Rep1)
	theirs := e.SeedPerson(t, "Their Contact", &e.Rep3)
	e.MakeCapturePrivate(t, "person", theirs, e.Rep3)
	seedLapsedPair(t, e, e.Rep1, mine, "the thread that lapsed")
	seedLapsedPair(t, e, e.Rep1, theirs, "their private thread")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphPerms)
	var quiet []search.InteractionEdge
	err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		var err error
		quiet, err = search.QuietEdgesForUser(
			rep, tx, time.Now().UTC().AddDate(0, 0, -relstrength.QuietDays), 10, nil)
		return err
	})
	if err != nil {
		t.Fatalf("reading quiet candidates: %v", err)
	}
	if len(quiet) != 1 {
		t.Fatalf("candidates = %d, want only the readable lapsed contact", len(quiet))
	}
	if quiet[0].PersonID != mine {
		t.Errorf("candidate is %s, want the caller's own contact %s", quiet[0].PersonID, mine)
	}
}

// The batched derivation filters the whole candidate set through the caller's
// row scope in one pass: an unreadable contact is absent from the answer, and
// the readable one arrives derived and named.
func TestBatchedChangesAdmitOnlyReadableContacts(t *testing.T) {
	e := Setup(t)
	mine := e.SeedPerson(t, "Anna Weber", &e.Rep1)
	theirs := e.SeedPerson(t, "Their Contact", &e.Rep3)
	e.MakeCapturePrivate(t, "person", theirs, e.Rep3)
	seedLapsedPair(t, e, e.Rep1, mine, "the thread that lapsed")
	seedLapsedPair(t, e, e.Rep1, theirs, "their private thread")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphPerms)
	store := people.NewStore(e.DB())
	var changed []people.PersonChanges
	err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		var err error
		changed, err = store.RelationshipChangesForPeople(rep, tx,
			[]ids.PersonID{ids.From[ids.PersonKind](mine), ids.From[ids.PersonKind](theirs)},
			time.Now().UTC())
		return err
	})
	if err != nil {
		t.Fatalf("deriving the candidate set: %v", err)
	}
	if len(changed) != 1 {
		t.Fatalf("derived contacts = %d, want only the readable one", len(changed))
	}
	if changed[0].PersonID.UUID != mine {
		t.Errorf("derived %s, want the caller's own contact %s", changed[0].PersonID.UUID, mine)
	}
	if changed[0].DisplayName != "Anna Weber" {
		t.Errorf("the contact is named %q, want the person record's own name", changed[0].DisplayName)
	}
	sawQuiet := false
	for _, change := range changed[0].Changes {
		if change.Kind == relstrength.ChangeWentQuiet {
			sawQuiet = true
		}
	}
	if !sawQuiet {
		t.Errorf("changes = %v, want the lapse the seeding arranged", changed[0].Changes)
	}
}

// A contact this reader set aside is removed BEFORE the candidate cap, so it
// cannot evict a real lapse from the budget.
//
// This is the whole reason the exclusion is a predicate the projection embeds
// rather than a filter over what it returned. The read takes the oldest
// silences up to a bound; a contact dropped afterwards has already spent a
// slot, so a rep who set several aside would lose real lapses off the bottom of
// their lane with nothing to say so. The waiting queue applies its own
// set-aside rule before its cap for the same reason.
//
// The cap is ONE here, which is what makes the two behaviours tell apart: with
// the exclusion before the cut the reader gets their live lapse, and with it
// after they get an empty lane.
func TestADismissedContactDoesNotSpendACandidateSlot(t *testing.T) {
	e := Setup(t)
	setAside := e.SeedPerson(t, "Dana Weiss", &e.Rep1)
	standing := e.SeedPerson(t, "Ines Sommer", &e.Rep1)
	// The dismissed contact went quiet LONGER ago, so it sorts first and is
	// exactly the row that would fill a cap of one.
	seedLapsedPairAt(t, e, e.Rep1, setAside, "the older thread", 120)
	seedLapsedPairAt(t, e, e.Rep1, standing, "the newer thread", 90)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, graphPerms)
	store := people.NewStore(database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS)))
	if err := store.DismissRelationshipNudge(rep, ids.From[ids.PersonKind](setAside), 30); err != nil {
		t.Fatalf("dismissing: %v", err)
	}

	now := time.Now().UTC()
	var quiet []search.InteractionEdge
	err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		var err error
		quiet, err = search.QuietEdgesForUser(
			rep, tx, now.AddDate(0, 0, -relstrength.QuietDays), 1,
			func(arg func(any) int) (string, error) {
				return people.NotDismissedClause(rep, "e", now, arg)
			})
		return err
	})
	if err != nil {
		t.Fatalf("reading quiet candidates: %v", err)
	}

	if len(quiet) != 1 {
		t.Fatalf("candidates = %d, want the one live lapse the budget has room for", len(quiet))
	}
	if quiet[0].PersonID != standing {
		t.Errorf("the candidate is %s, want %s — the dismissed contact spent the slot",
			quiet[0].PersonID, standing)
	}
}
