// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package introductions

// What the Network tab may know about a route it did not ask about.
//
// The property this file exists for cannot be seen without Postgres: the read
// is deliberately NOT scoped to the caller's own asks, because the duplicate
// guard is not either. A rep must be told the door is taken by a colleague
// whose ask they cannot open — otherwise the tab offers a route the index
// refuses, which is the whole defect the read was written to fix.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func directRoute(introducer ids.UUID) RouteKey {
	return RouteKey{Introducer: ids.From[ids.UserKind](introducer)}
}

// A rep who is party to NEITHER side of an ask still learns the route is
// taken. Scoping this read the way the ask's own reads are scoped would tell
// this rep the door is free while the index holds it shut, and they would find
// out by being refused after writing the whole ask.
func TestAnotherRepsOpenAskTakesTheRoute(t *testing.T) {
	e := setupIntro(t)
	if _, err := e.store.Create(e.asUser(e.requester), e.ask()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	taken, err := e.store.RouteStates(
		e.asUser(e.stranger), ids.From[ids.PersonKind](e.contact))
	if err != nil {
		t.Fatalf("RouteStates: %v", err)
	}
	if got := taken[directRoute(e.introducer)]; got != RouteOpen {
		t.Errorf("a third rep reads the route as %q; the duplicate guard would "+
			"refuse their ask, so the tab must show it taken", got)
	}
}

// And the ask itself stays private. The state is the whole payload: that the
// door is taken is the fact this rep needs, while who asked and what they
// wrote is between those two.
func TestTheStateIsAllAThirdRepLearns(t *testing.T) {
	e := setupIntro(t)
	if _, err := e.store.Create(e.asUser(e.requester), e.ask()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	asks, err := e.store.ForPerson(
		e.asUser(e.stranger), e.contact, 20)
	if err != nil {
		t.Fatalf("ForPerson: %v", err)
	}
	if len(asks) != 0 {
		t.Errorf("a third rep read %d of somebody else's asks; the route state "+
			"is all they may learn", len(asks))
	}
}

// A refusal is reported as a refusal and not as an open ask. The guard is one
// OPEN ask, so a declined route is still askable — the tab says the colleague
// said no before, and lets the rep decide.
func TestARefusedRouteReportsRefusedAndStaysAskable(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := e.store.Decide(
		e.asUser(e.introducer), id, StatusDeclined, "Not close enough to help.", nil, 1,
	); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	taken, err := e.store.RouteStates(
		e.asUser(e.requester), ids.From[ids.PersonKind](e.contact))
	if err != nil {
		t.Fatalf("RouteStates: %v", err)
	}
	if got := taken[directRoute(e.introducer)]; got != RouteRefused {
		t.Errorf("a declined route reads %q; want refused", got)
	}
	// And the server agrees the route is free to ask again, which is why the
	// tab shows this state rather than blocking on it.
	if _, err := e.store.Create(e.asUser(e.requester), e.ask()); err != nil {
		t.Errorf("a fresh ask after a refusal was rejected: %v", err)
	}
}

// A live ask outranks an old refusal on the same route: the rep can act on
// neither, and the live one is the truer sentence.
func TestALiveAskOutranksAnEarlierRefusal(t *testing.T) {
	e := setupIntro(t)
	first, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := e.store.Decide(
		e.asUser(e.introducer), first, StatusDeclined, "Not this quarter.", nil, 1,
	); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if _, err := e.store.Create(e.asUser(e.requester), e.ask()); err != nil {
		t.Fatalf("second Create: %v", err)
	}

	taken, err := e.store.RouteStates(
		e.asUser(e.requester), ids.From[ids.PersonKind](e.contact))
	if err != nil {
		t.Fatalf("RouteStates: %v", err)
	}
	if got := taken[directRoute(e.introducer)]; got != RouteOpen {
		t.Errorf("a route with a live ask and an older refusal reads %q; "+
			"the live ask is what the rep cannot act around", got)
	}
}

// A settled ask stops holding its route: the tab offers it again, exactly as
// the guard index does.
func TestASettledAskReleasesItsRoute(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := e.store.Cancel(e.asUser(e.requester), id, "Went in another way.", 1); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	taken, err := e.store.RouteStates(
		e.asUser(e.requester), ids.From[ids.PersonKind](e.contact))
	if err != nil {
		t.Fatalf("RouteStates: %v", err)
	}
	if state, held := taken[directRoute(e.introducer)]; held {
		t.Errorf("a withdrawn ask still reports %q, holding a route the guard "+
			"index has already released", state)
	}
}

// The contact gate rides this read, because it discloses about them. Without
// it a rep could probe which colleagues have been asked about a person they
// may not open.
func TestRouteStatesRefusesAContactTheCallerCannotSee(t *testing.T) {
	e := setupIntro(t)

	_, err := e.store.RouteStates(
		e.asUser(e.requester), ids.From[ids.PersonKind](ids.NewV7()))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("route states for a contact that does not exist gave %v; want not-found", err)
	}

	if _, err := e.owner.Exec(context.Background(),
		`UPDATE person SET archived_at = now() WHERE id = $1`, e.unseen); err != nil {
		t.Fatal(err)
	}
	_, err = e.store.RouteStates(e.asUser(e.requester), ids.From[ids.PersonKind](e.unseen))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("route states for an erased contact gave %v; want not-found", err)
	}

	// The admit case, without which both refusals above would pass against a
	// read that refused every contact.
	if _, err := e.store.RouteStates(
		e.asUser(e.requester), ids.From[ids.PersonKind](e.contact)); err != nil {
		t.Errorf("a live contact's route states could not be read: %v", err)
	}
}

// No seat, no answer. This read is blind by design — it reports on asks the
// caller is not party to — and that trade is only safe while a person is
// behind it.
func TestRouteStatesNeedsAPersonBehindIt(t *testing.T) {
	e := setupIntro(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:none",
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"introduction": {Read: true}, "person": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	_, err := e.store.RouteStates(ctx, ids.From[ids.PersonKind](e.contact))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a caller with no seat read route states (%v)", err)
	}
}
