// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package introductions

// The clock closing an ask nobody answered.
//
// Its own file rather than more of store_integration_test.go: the sweep is one
// concern with its own actor, and the two properties that need a real database
// are the ones a unit test cannot reach — the partial unique index releasing
// the route, and the write shape's audit and outbox rows.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asClock is the sweep's own principal: the system actor whose name every
// audit row it writes carries.
func (e *introEnv) asClock() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: ExpiryActor,
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// asSomeOtherSystem is a system principal that is NOT the sweep.
//
// The refusal checks the actor ID as well as the type, because "some system
// principal" is not the claim being made: every audit row this pass writes is
// attributed to ExpiryActor, and a caller who cannot present that id would be
// closing introductions under a name that is not theirs.
func (e *introEnv) asSomeOtherSystem() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:something-else",
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// contactFor seeds a contact of its own, so two asks on one colleague do not
// meet the duplicate guard.
func (e *introEnv) contactFor(t *testing.T, label string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO person (id, full_name, source, captured_by, owner_id)
		 VALUES ($1, $2, 'manual', 'test', $3)`,
		id, "Contact "+label, e.requester); err != nil {
		t.Fatal(err)
	}
	return id
}

// overdue backdates an ask's deadline so the sweep sees it.
//
// Written straight to the column because there is no product path that moves a
// due date backwards — the alternative is a test that sleeps for a week.
func (e *introEnv) overdue(t *testing.T, id ids.UUID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE intro_request SET due_at = $2 WHERE id = $1`,
		id, testNow.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
}

// The whole point: an unanswered ask closes, and the route it was holding is
// free again.
//
// The second half is what makes this more than tidiness. The open-route index
// is a partial unique index, so an ask that can never leave an open status
// holds that (contact, colleague) pair permanently — the duplicate guard, which
// exists to stop two tabs racing, becomes a refusal the rep can never clear.
func TestAnUnansweredAskExpiresAndFreesItsRoute(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	e.overdue(t, id)

	expired, err := e.store.ExpireDue(e.asClock())
	if err != nil {
		t.Fatalf("ExpireDue: %v", err)
	}
	if expired != 1 {
		t.Fatalf("the sweep closed %d ask(s); want one", expired)
	}

	var status string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status FROM intro_request WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(StatusExpired) {
		t.Errorf("the ask is %q after its deadline passed; want expired", status)
	}

	// And the route is askable again. Before the sweep existed this second
	// Create failed on the index forever.
	if _, err := e.store.Create(e.asUser(e.requester), e.ask()); err != nil {
		t.Errorf("the route is still blocked after the ask expired: %v", err)
	}
}

// An expiry is a decision, so it writes what every other decision writes.
//
// The trail has to say which state ran out of time: an ask nobody answered and
// one a colleague accepted and then dropped are different stories about that
// colleague, and only the before-image tells them apart.
func TestAnExpiryLandsInTheWriteShapeUnderTheClocksName(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := e.store.Decide(
		e.asUser(e.introducer), id, StatusAccepted, "", nil, 1); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	e.overdue(t, id)

	if _, err := e.store.ExpireDue(e.asClock()); err != nil {
		t.Fatalf("ExpireDue: %v", err)
	}

	ctx := context.Background()
	var before, actorType, actorID string
	if err := e.owner.QueryRow(ctx, `
		SELECT before->>'status', actor_type, actor_id FROM audit_log
		 WHERE entity_type = 'intro_request' AND entity_id = $1
		   AND after->>'status' = 'expired'`, id).Scan(&before, &actorType, &actorID); err != nil {
		t.Fatal(err)
	}
	if before != string(StatusAccepted) {
		t.Errorf("the trail says the expiry followed %q; want accepted — an ask "+
			"nobody answered and one a colleague dropped are different stories", before)
	}
	// The clock's name, not a person's. A human's id here would put their name
	// on a refusal they never made — and the TYPE matters as much as the id,
	// because that is what a reader scans to tell an automated close from a
	// colleague's decision.
	if actorType != "system" || actorID != ExpiryActor {
		t.Errorf("the audit row is attributed to %s:%q; want system:%q",
			actorType, actorID, ExpiryActor)
	}

	var events int
	if err := e.owner.QueryRow(ctx, `
		SELECT count(*) FROM event_outbox
		 WHERE envelope->>'type' = 'intro_request.closed'
		   AND envelope->'payload'->>'reason' = 'expired'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Errorf("%d expiry events; want one — a rep withdrawing and a queue "+
			"timing out call for different follow-ups", events)
	}
}

// Expiry reaches every state where somebody still owes an action.
//
// An accepted ask nobody completed is exactly the request a queue loses
// quietly: the colleague said yes, nothing happened, and the record still reads
// as a promise being kept.
func TestExpiryReachesAnAcceptedAskAndALentName(t *testing.T) {
	e := setupIntro(t)
	for _, answer := range []Status{StatusAccepted, StatusNameDropApproved} {
		ask := e.ask()
		// A contact of its own, so the duplicate guard admits both asks.
		ask.PersonID = e.contactFor(t, string(answer))
		id, err := e.store.Create(e.asUser(e.requester), ask)
		if err != nil {
			t.Fatalf("Create for %s: %v", answer, err)
		}
		if err := e.store.Decide(e.asUser(e.introducer), id, answer, "", nil, 1); err != nil {
			t.Fatalf("Decide %s: %v", answer, err)
		}
		e.overdue(t, id)
	}

	expired, err := e.store.ExpireDue(e.asClock())
	if err != nil {
		t.Fatalf("ExpireDue: %v", err)
	}
	if expired != 2 {
		t.Errorf("the sweep closed %d ask(s); want both the accepted one and the "+
			"lent name — an accepted ask nobody completed is the one a queue "+
			"loses quietly", expired)
	}
}

// A settled ask is left exactly as its answerer left it.
//
// Re-closing a decided ask would rewrite its outcome, and the audit trail would
// carry two endings for one question.
func TestTheSweepLeavesASettledAskAlone(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := e.store.Decide(
		e.asUser(e.introducer), id, StatusDeclined, "not close enough", nil, 1); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	e.overdue(t, id)

	expired, err := e.store.ExpireDue(e.asClock())
	if err != nil {
		t.Fatalf("ExpireDue: %v", err)
	}
	if expired != 0 {
		t.Errorf("the sweep closed %d settled ask(s); want none", expired)
	}
	var status string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status FROM intro_request WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != string(StatusDeclined) {
		t.Errorf("a declined ask became %q — the colleague's answer was rewritten", status)
	}
}

// An ask still inside its window is not touched.
//
// Without this, every other case here would pass against a sweep that closed
// every open ask regardless of the clock.
func TestTheSweepLeavesAnAskInsideItsWindow(t *testing.T) {
	e := setupIntro(t)
	if _, err := e.store.Create(e.asUser(e.requester), e.ask()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	expired, err := e.store.ExpireDue(e.asClock())
	if err != nil {
		t.Fatalf("ExpireDue: %v", err)
	}
	if expired != 0 {
		t.Errorf("the sweep closed %d ask(s) that still had time; want none", expired)
	}
}

// Only the clock may run the sweep.
//
// This closes asks in bulk without consulting anybody's row scope. Left open it
// would be an authenticated user's way to cancel every open introduction in the
// installation at once — each one audited as though the clock had done it, with
// their name nowhere in the record.
func TestOnlyTheClockMayExpireIntroductions(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	e.overdue(t, id)

	for name, ctx := range map[string]context.Context{
		"the requester":  e.asUser(e.requester),
		"the introducer": e.asUser(e.introducer),
		"a stranger":     e.asUser(e.stranger),
		"another system": e.asSomeOtherSystem(),
	} {
		if _, err := e.store.ExpireDue(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s could run the expiry sweep (%v)", name, err)
		}
	}

	// The admit case, without which every refusal above would pass against a
	// sweep that refused everyone.
	if _, err := e.store.ExpireDue(e.asClock()); err != nil {
		t.Errorf("the clock itself was refused: %v", err)
	}
}
