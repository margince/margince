// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Who a quiet-relationship suggestion is for, against the real projection,
// derivation and dismissal store. The contact's owner gets it. A colleague who
// wrote the mail or was copied on it does not. A contact without a live owner
// is nobody's personal work.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// quietLaneNow is the fixed read instant; every exchange is dated from it.
var quietLaneNow = time.Date(2026, time.March, 10, 9, 0, 0, 0, time.UTC)

// seedQuietExchange records two mails between a sender and a contact, with a
// copied colleague, and folds them through the real projection writer.
func seedQuietExchange(t *testing.T, e *integration.Env, sender, copied, contact ids.UUID, daysAgo int) {
	t.Helper()
	owner := integration.OwnerConn(t)
	defer func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing fixture connection: %v", err)
		}
	}()
	ctx := context.Background()
	last := quietLaneNow.AddDate(0, 0, -daysAgo)
	var folded []ids.UUID
	for _, at := range []time.Time{last.AddDate(0, 0, -1), last} {
		activity := ids.NewV7()
		if _, err := owner.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
			VALUES ($1, 'email', 'checking in', $2, 'outbound', 'manual', 'human:x')`, activity, at); err != nil {
			t.Fatal(err)
		}
		integration.LinkActivity(t, owner, activity, "contact", contact)
		for _, party := range []struct {
			column string
			id     ids.UUID
			role   string
		}{{"user_id", sender, "from"}, {"contact_id", contact, "to"}, {"user_id", copied, "cc"}} {
			if party.id.IsZero() {
				continue
			}
			if _, err := owner.Exec(ctx, `INSERT INTO activity_participant (activity_id, `+party.column+`, role) VALUES ($1, $2, $3)`,
				activity, party.id, party.role); err != nil {
				t.Fatal(err)
			}
		}
		folded = append(folded, activity)
	}
	wsCtx := principal.WithWorkspaceID(ctx, e.WS)
	if err := database.WithWorkspaceTx(wsCtx, e.Pool, func(tx pgx.Tx) error {
		return search.RecomputeEdgesForActivities(wsCtx, tx, folded)
	}); err != nil {
		t.Fatalf("folding the exchange into the projection: %v", err)
	}
}

func quietLaneFor(t *testing.T, e *integration.Env, reader ids.UUID, scope attention.TaskScope, owner ids.UUID) []attention.QuietRelationship {
	t.Helper()
	return quietLaneAt(t, e, quietLaneNow, reader, scope, owner)
}

func quietLaneAt(t *testing.T, e *integration.Env, now time.Time, reader ids.UUID, scope attention.TaskScope, owner ids.UUID) []attention.QuietRelationship {
	t.Helper()
	rows, _ := quietLaneWithBound(t, e, now, reader, scope, owner)
	return rows
}

func quietLaneWithBound(t *testing.T, e *integration.Env, now time.Time, reader ids.UUID, scope attention.TaskScope, owner ids.UUID) ([]attention.QuietRelationship, bool) {
	t.Helper()
	lane := attentionDecay{pool: e.Pool, store: contacts.NewStore(e.DB()), now: func() time.Time { return now }}
	rows, more, err := lane.Lapsed(e.As(reader, nil, integration.AdminPerms), scope, owner)
	if err != nil {
		t.Fatalf("reading the quiet lane: %v", err)
	}
	return rows, more
}

func TestTheQuietSuggestionGoesToTheContactOwnerNotToWhoWroteOrWasCopied(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Paula Quiet", &e.Rep1)
	seedQuietExchange(t, e, e.Rep2, e.Rep3, contact, 60)

	got := quietLaneFor(t, e, e.Rep1, attention.TasksMine, ids.UUID{})
	if len(got) != 1 || got[0].ContactID != contact || got[0].OwnerID != e.Rep1 {
		t.Fatalf("the owner's lane = %+v, want the quiet contact owned by them", got)
	}
	for _, other := range []ids.UUID{e.Rep2, e.Rep3} {
		if got := quietLaneFor(t, e, other, attention.TasksMine, ids.UUID{}); len(got) != 0 {
			t.Errorf("%s (sender or copied) got %+v in their own lane, want nothing", other, got)
		}
	}
}

func TestAQuietContactWithADepartedOwnerIsNobodysPersonalWork(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Left Behind", &e.Rep1)
	seedQuietExchange(t, e, e.Rep2, ids.UUID{}, contact, 60)
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.Rep1); err != nil {
		t.Fatal(err)
	}

	if got := quietLaneFor(t, e, e.Rep2, attention.TasksMine, ids.UUID{}); len(got) != 0 {
		t.Errorf("the sender's own lane = %+v, want nothing: no fallback owner", got)
	}
	got := quietLaneFor(t, e, e.Rep2, attention.TasksUnassigned, ids.UUID{})
	if len(got) != 1 || got[0].ContactID != contact || !got[0].OwnerID.IsZero() {
		t.Fatalf("the unassigned lane = %+v, want the contact with no owner", got)
	}
}

func TestAQuietRelationshipPastOneHundredEightyDaysLeavesTheWorklist(t *testing.T) {
	e := integration.Setup(t)
	recent := e.SeedContact(t, "Still Warm", &e.Rep1)
	old := e.SeedContact(t, "Imported Years Ago", &e.Rep1)
	seedQuietExchange(t, e, e.Rep1, ids.UUID{}, recent, 179)
	seedQuietExchange(t, e, e.Rep1, ids.UUID{}, old, 400)

	got := quietLaneFor(t, e, e.Rep1, attention.TasksMine, ids.UUID{})
	if len(got) != 1 || got[0].ContactID != recent {
		t.Fatalf("the lane = %+v, want only the relationship inside 180 days", got)
	}
}

func TestAPermanentStopOutlastsNinetyDaysAndResumes(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Over For Good", &e.Rep1)
	seedQuietExchange(t, e, e.Rep1, ids.UUID{}, contact, 60)
	rep := e.As(e.Rep1, nil, integration.AdminPerms)
	store := contacts.NewStore(e.DB()).WithClock(func() time.Time { return quietLaneNow })
	id := ids.From[ids.ContactKind](contact)
	if err := store.DismissRelationshipNudge(rep, id, 7); err != nil {
		t.Fatalf("dismissing for a week: %v", err)
	}
	if err := store.DismissRelationshipNudges(rep, []ids.ContactID{id}, contacts.NudgeSpan{Permanent: true}); err != nil {
		t.Fatalf("stopping for good: %v", err)
	}
	later := quietLaneNow.AddDate(0, 0, 91)
	if got := quietLaneAt(t, e, later, e.Rep1, attention.TasksMine, ids.UUID{}); len(got) != 0 {
		t.Fatalf("the lane 91 days on = %+v after a permanent stop, want nothing", got)
	}
	if got := quietLaneFor(t, e, e.Rep2, attention.TasksOwnedBy, e.Rep1); len(got) != 1 {
		t.Errorf("a colleague viewing the owner's day = %+v, want their own choice, not the owner's stop", got)
	}
	if err := store.RestoreRelationshipNudge(rep, id); err != nil {
		t.Fatalf("resuming: %v", err)
	}
	if got := quietLaneAt(t, e, later, e.Rep1, attention.TasksMine, ids.UUID{}); len(got) != 1 {
		t.Fatalf("the lane = %+v after resuming, want the contact back", got)
	}
}

func TestDismissAllSetsEveryListedContactAsideOrNone(t *testing.T) {
	e := integration.Setup(t)
	first := e.SeedContact(t, "First Quiet", &e.Rep1)
	second := e.SeedContact(t, "Second Quiet", &e.Rep1)
	seedQuietExchange(t, e, e.Rep1, ids.UUID{}, first, 60)
	seedQuietExchange(t, e, e.Rep1, ids.UUID{}, second, 70)
	rep := e.As(e.Rep1, nil, integration.AdminPerms)
	store := contacts.NewStore(e.DB()).WithClock(func() time.Time { return quietLaneNow })

	missing := ids.From[ids.ContactKind](ids.NewV7())
	if err := store.DismissRelationshipNudges(rep,
		[]ids.ContactID{ids.From[ids.ContactKind](first), missing}, contacts.NudgeSpan{Days: 30}); err == nil {
		t.Fatal("a bulk dismissal naming an unknown contact succeeded")
	}
	if got := quietLaneFor(t, e, e.Rep1, attention.TasksMine, ids.UUID{}); len(got) != 2 {
		t.Fatalf("after a refused bulk dismissal the lane = %+v, want both contacts still there", got)
	}
	if err := store.DismissRelationshipNudges(rep,
		[]ids.ContactID{ids.From[ids.ContactKind](first), ids.From[ids.ContactKind](second)}, contacts.NudgeSpan{Days: 30}); err != nil {
		t.Fatalf("dismissing all: %v", err)
	}
	if got := quietLaneFor(t, e, e.Rep1, attention.TasksMine, ids.UUID{}); len(got) != 0 {
		t.Fatalf("after dismiss all the lane = %+v, want nothing", got)
	}
}

// seedRejectedCandidate gives a contact three old exchanges, which rank it
// ahead of a two-exchange contact, and a recent linked call nobody took part
// in. The edges stay old, so the scan offers it; the contact-wide derivation
// sees the call and rejects the silence.
func seedRejectedCandidate(t *testing.T, e *integration.Env, owner ids.UUID, name string) {
	t.Helper()
	contact := e.SeedContact(t, name, &owner)
	seedQuietExchange(t, e, owner, ids.UUID{}, contact, 60)
	seedQuietExchange(t, e, owner, ids.UUID{}, contact, 70)
	call := ids.NewV7()
	conn := integration.OwnerConn(t)
	defer func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing fixture connection: %v", err)
		}
	}()
	if _, err := conn.Exec(context.Background(), `
		INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'call', 'catch-up', $2, 'manual', 'human:x')`, call, quietLaneNow.AddDate(0, 0, -5)); err != nil {
		t.Fatal(err)
	}
	integration.LinkActivity(t, conn, call, "contact", contact)
}

func TestAValidQuietContactBehindAFullPageOfRejectedCandidatesIsReached(t *testing.T) {
	e := integration.Setup(t)
	for i := range 40 {
		seedRejectedCandidate(t, e, e.Rep1, fmt.Sprintf("Recently called %02d", i))
	}
	valid := e.SeedContact(t, "Truly quiet", &e.Rep1)
	seedQuietExchange(t, e, e.Rep1, ids.UUID{}, valid, 60)

	rows, more := quietLaneWithBound(t, e, quietLaneNow, e.Rep1, attention.TasksMine, ids.UUID{})
	if len(rows) != 1 || rows[0].ContactID != valid {
		t.Fatalf("the lane = %+v, want the 41st candidate that is really quiet", rows)
	}
	if more {
		t.Error("the scan read to the end of the candidates and still said more may exist")
	}
}

func TestAQuietScanThatSpendsItsBudgetSaysSoWithNoSurvivors(t *testing.T) {
	e := integration.Setup(t)
	for i := range decayCandidateCap * decayCandidatePages {
		seedRejectedCandidate(t, e, e.Rep1, fmt.Sprintf("Recently called %03d", i))
	}
	rows, more := quietLaneWithBound(t, e, quietLaneNow, e.Rep1, attention.TasksMine, ids.UUID{})
	if len(rows) != 0 || !more {
		t.Fatalf("the lane = %d rows, more = %v; want none and a lane marked as possibly more", len(rows), more)
	}
}

// A stop is one reader's choice: it reads back as stopped for them, as
// following for a colleague, and as following again once resumed.
func TestAStopReadsBackPerReaderAndClearsOnResume(t *testing.T) {
	e := integration.Setup(t)
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Per Reader", &e.Rep1))
	store := contacts.NewStore(e.DB()).WithClock(func() time.Time { return quietLaneNow })
	reader := e.As(e.Rep1, nil, integration.AdminPerms)
	colleague := e.As(e.Rep2, nil, integration.AdminPerms)
	if err := store.DismissRelationshipNudges(reader, []ids.ContactID{contact}, contacts.NudgeSpan{Permanent: true}); err != nil {
		t.Fatal(err)
	}
	if got, err := store.RelationshipNudgeState(reader, contact); err != nil || !got.Stopped {
		t.Fatalf("the reader's state = %+v, %v, want stopped", got, err)
	}
	if got, err := store.RelationshipNudgeState(colleague, contact); err != nil || got.Stopped || got.Until != nil {
		t.Fatalf("a colleague's state = %+v, %v, want following", got, err)
	}
	if err := store.RestoreRelationshipNudge(reader, contact); err != nil {
		t.Fatal(err)
	}
	if got, err := store.RelationshipNudgeState(reader, contact); err != nil || got.Stopped || got.Until != nil {
		t.Fatalf("after resume the reader's state = %+v, %v, want following", got, err)
	}
}
