// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import (
	"context"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// somePerson is the contact a graph is about; the stub ignores it.
var somePerson = ids.From[ids.PersonKind](ids.UUID(uuidFor(9)))

// routesToStamp is two routes through the SAME colleague — one direct, one
// through a contact — which is the pair the whole keying rule is about.
func routesToStamp() []crmcontracts.PersonGraphRouteCandidate {
	through := openapi_types.UUID(uuidFor(3))
	return []crmcontracts.PersonGraphRouteCandidate{
		{
			RouteId:      "direct:1",
			ViaUserId:    openapi_types.UUID(uuidFor(1)),
			Availability: crmcontracts.PersonGraphRouteAvailabilityAvailable,
		},
		{
			RouteId:         "through:1:3",
			ViaUserId:       openapi_types.UUID(uuidFor(1)),
			ThroughPersonId: &through,
			Availability:    crmcontracts.PersonGraphRouteAvailabilityAvailable,
		},
	}
}

func directKey(user byte) introductions.RouteKey {
	return introductions.RouteKey{Introducer: ids.From[ids.UserKind](ids.UUID(uuidFor(user)))}
}

func throughKey(user, contact byte) introductions.RouteKey {
	return introductions.RouteKey{
		Introducer: ids.From[ids.UserKind](ids.UUID(uuidFor(user))),
		Through:    ids.From[ids.PersonKind](ids.UUID(uuidFor(contact))),
	}
}

// An open ask greys out the route it was made on, and the rep is told before
// they write the ask rather than by the 409 after it.
func TestAnOpenAskMarksItsOwnRoute(t *testing.T) {
	routes := routesToStamp()
	stampAvailability(routes, map[introductions.RouteKey]introductions.RouteState{directKey(1): introductions.RouteOpen})

	if got := routes[0].Availability; got != crmcontracts.PersonGraphRouteAvailabilityAlreadyRequested {
		t.Errorf("the route with a live ask reads %q; want already_requested", got)
	}
}

// The duplicate guard keys on the colleague AND the intermediary, so asking
// Lena directly leaves asking Lena through Marco open. Greying both out would
// refuse an ask the server accepts, and the rep would never try it.
func TestAnOpenDirectAskLeavesTheIndirectRouteFree(t *testing.T) {
	routes := routesToStamp()
	stampAvailability(routes, map[introductions.RouteKey]introductions.RouteState{directKey(1): introductions.RouteOpen})

	if got := routes[1].Availability; got != crmcontracts.PersonGraphRouteAvailabilityAvailable {
		t.Errorf("the indirect route through the same colleague reads %q; "+
			"the duplicate guard would have accepted it", got)
	}
}

// And the reverse, so the key is proven to carry the intermediary rather than
// merely to tolerate it.
func TestAnOpenIndirectAskLeavesTheDirectRouteFree(t *testing.T) {
	routes := routesToStamp()
	stampAvailability(routes, map[introductions.RouteKey]introductions.RouteState{throughKey(1, 3): introductions.RouteOpen})

	if got := routes[1].Availability; got != crmcontracts.PersonGraphRouteAvailabilityAlreadyRequested {
		t.Errorf("the indirect route with a live ask reads %q; want already_requested", got)
	}
	if got := routes[0].Availability; got != crmcontracts.PersonGraphRouteAvailabilityAvailable {
		t.Errorf("the direct route reads %q; the ask was on the indirect one", got)
	}
}

// A refusal is not a bar — the guard is one OPEN ask — but the tab says so,
// which is what stops the product recommending a door that was closed once.
func TestARefusalIsShownAsDeclined(t *testing.T) {
	routes := routesToStamp()
	stampAvailability(routes, map[introductions.RouteKey]introductions.RouteState{directKey(1): introductions.RouteRefused})

	if got := routes[0].Availability; got != crmcontracts.PersonGraphRouteAvailabilityDeclined {
		t.Errorf("a previously refused route reads %q; want declined", got)
	}
}

// Nothing known against a route leaves it exactly as the graph produced it.
func TestAnUntouchedRouteStaysAvailable(t *testing.T) {
	routes := routesToStamp()
	stampAvailability(routes, map[introductions.RouteKey]introductions.RouteState{directKey(7): introductions.RouteOpen})

	for i, route := range routes {
		if route.Availability != crmcontracts.PersonGraphRouteAvailabilityAvailable {
			t.Errorf("route %d reads %q with no ask against it; want available", i, route.Availability)
		}
	}
}

// askedStub stands in for the introductions reader at the seam.
type askedStub struct {
	states map[introductions.RouteKey]introductions.RouteState
	err    error
}

func (s askedStub) RouteStates(context.Context, ids.PersonID) (map[introductions.RouteKey]introductions.RouteState, error) {
	return s.states, s.err
}

func graphWithRoutes() *crmcontracts.PersonGraph {
	routes := routesToStamp()
	return &crmcontracts.PersonGraph{Routes: &routes}
}

// The reader's answer reaches the payload. Without this the seam could be
// wired and stamping nothing, and every route would read available exactly as
// it did before the seam existed.
func TestTheReadersAnswerReachesTheGraph(t *testing.T) {
	out := graphWithRoutes()
	reads := Reads{}.WithAskedRoutes(askedStub{
		states: map[introductions.RouteKey]introductions.RouteState{directKey(1): introductions.RouteOpen},
	})

	if err := reads.markAskedRoutes(context.Background(), somePerson, out); err != nil {
		t.Fatalf("markAskedRoutes: %v", err)
	}
	if got := (*out.Routes)[0].Availability; got != crmcontracts.PersonGraphRouteAvailabilityAlreadyRequested {
		t.Errorf("the route reads %q; the reader said an ask was live on it", got)
	}
}

// A caller with no introduction grant is served the graph with every route
// offerable: they hold no ask that could collide, and the one they cannot make
// is refused where they make it.
func TestADeniedReaderLeavesTheGraphStanding(t *testing.T) {
	out := graphWithRoutes()
	reads := Reads{}.WithAskedRoutes(askedStub{err: apperrors.ErrPermissionDenied})

	if err := reads.markAskedRoutes(context.Background(), somePerson, out); err != nil {
		t.Fatalf("a denial failed the graph read: %v", err)
	}
	if got := (*out.Routes)[0].Availability; got != crmcontracts.PersonGraphRouteAvailabilityAvailable {
		t.Errorf("a denied reader left the route reading %q; want available", got)
	}
}

// The contact gated twice, and archived between the two: the graph was already
// served, so the routes go unstamped rather than a 404 replacing a valid page.
func TestAContactArchivedMidRequestLeavesTheGraphStanding(t *testing.T) {
	out := graphWithRoutes()
	reads := Reads{}.WithAskedRoutes(askedStub{err: apperrors.ErrNotFound})

	if err := reads.markAskedRoutes(context.Background(), somePerson, out); err != nil {
		t.Fatalf("a mid-request archive failed the whole graph read: %v", err)
	}
}

// Any other fault fails the read. A graph that quietly claims every door is
// open is worse than an error, because the rep acts on it.
func TestAFailedReaderFailsTheRead(t *testing.T) {
	out := graphWithRoutes()
	broken := errors.New("the introductions read fell over")
	reads := Reads{}.WithAskedRoutes(askedStub{err: broken})

	if err := reads.markAskedRoutes(context.Background(), somePerson, out); !errors.Is(err, broken) {
		t.Errorf("a broken reader gave %v; the graph must not claim every route is free", err)
	}
}

// No reader wired is an ordinary configuration, not a failure: the routes are
// served exactly as the graph produced them.
func TestNoReaderLeavesEveryRouteAsTheGraphMadeIt(t *testing.T) {
	out := graphWithRoutes()
	if err := (Reads{}).markAskedRoutes(context.Background(), somePerson, out); err != nil {
		t.Fatalf("an unwired reader failed the read: %v", err)
	}
	if got := (*out.Routes)[0].Availability; got != crmcontracts.PersonGraphRouteAvailabilityAvailable {
		t.Errorf("route reads %q with no reader wired; want available", got)
	}
}
