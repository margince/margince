// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

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

func directKey(user byte) RouteKey {
	return RouteKey{Introducer: ids.From[ids.UserKind](ids.UUID(uuidFor(user)))}
}

func throughKey(user, contact byte) RouteKey {
	return RouteKey{
		Introducer: ids.From[ids.UserKind](ids.UUID(uuidFor(user))),
		Through:    ids.From[ids.PersonKind](ids.UUID(uuidFor(contact))),
	}
}

// An open ask greys out the route it was made on, and the rep is told before
// they write the ask rather than by the 409 after it.
func TestAnOpenAskMarksItsOwnRoute(t *testing.T) {
	routes := routesToStamp()
	stampAvailability(routes, map[RouteKey]RouteState{directKey(1): RouteOpen})

	if got := routes[0].Availability; got != crmcontracts.PersonGraphRouteAvailabilityAlreadyRequested {
		t.Errorf("the route with a live ask reads %q; want already_requested", got)
	}
}

// The duplicate guard keys on the colleague AND the intermediary, so asking
// Lena directly leaves asking Lena through Marco open. Greying both out would
// refuse an ask the server accepts, and the rep would never try it.
func TestAnOpenDirectAskLeavesTheIndirectRouteFree(t *testing.T) {
	routes := routesToStamp()
	stampAvailability(routes, map[RouteKey]RouteState{directKey(1): RouteOpen})

	if got := routes[1].Availability; got != crmcontracts.PersonGraphRouteAvailabilityAvailable {
		t.Errorf("the indirect route through the same colleague reads %q; "+
			"the duplicate guard would have accepted it", got)
	}
}

// And the reverse, so the key is proven to carry the intermediary rather than
// merely to tolerate it.
func TestAnOpenIndirectAskLeavesTheDirectRouteFree(t *testing.T) {
	routes := routesToStamp()
	stampAvailability(routes, map[RouteKey]RouteState{throughKey(1, 3): RouteOpen})

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
	stampAvailability(routes, map[RouteKey]RouteState{directKey(1): RouteRefused})

	if got := routes[0].Availability; got != crmcontracts.PersonGraphRouteAvailabilityDeclined {
		t.Errorf("a previously refused route reads %q; want declined", got)
	}
}

// Nothing known against a route leaves it exactly as the graph produced it.
func TestAnUntouchedRouteStaysAvailable(t *testing.T) {
	routes := routesToStamp()
	stampAvailability(routes, map[RouteKey]RouteState{directKey(7): RouteOpen})

	for i, route := range routes {
		if route.Availability != crmcontracts.PersonGraphRouteAvailabilityAvailable {
			t.Errorf("route %d reads %q with no ask against it; want available", i, route.Availability)
		}
	}
}
