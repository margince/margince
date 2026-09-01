// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

// A route the server would refuse says so before the rep writes the ask.
//
// Every candidate the graph produces is offerable as far as the graph knows —
// it ranks relationships and has no idea which favours have already been
// asked. The introductions module holds that, and refuses a second open ask on
// one route with a 409. Until the two are joined the tab offers a door that is
// not there: the rep picks the route, writes the reason, writes the note,
// presses send, and the refusal arrives after all the work rather than before
// any of it.
//
// So the graph read asks, once, which routes to this contact are spoken for,
// and stamps the candidates. Nil is an ordinary configuration and not a
// failure: unstamped, every route reads `available`, which is what shipped
// before this seam existed.
//
// One value the enum carries is NOT set here. `unavailable` means the seat can
// no longer carry the ask, and a colleague in that state never reaches this
// code: EdgesForPerson joins app_user on the live-member predicate, so a
// deactivated colleague is not a graph node and so not a candidate. Writing a
// branch for it would be writing a branch that cannot run.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AskedRoutes reports which routes to a contact are already spoken for.
//
// An interface rather than the store itself, so the stamping can be tested
// against a reader that answers whatever the case needs. It speaks the
// introductions module's own types: a second vocabulary here would be two
// spellings of one fact, and the translation between them is exactly where a
// new state gets dropped and reads as "route is free".
//
// The map is keyed by route and states only that — no requester, no reason, no
// date — so a rep learns a door is taken without learning who took it.
type AskedRoutes interface {
	RouteStates(
		ctx context.Context, personID ids.PersonID,
	) (map[introductions.RouteKey]introductions.RouteState, error)
}

// WithAskedRoutes binds the reader that knows which routes are taken.
func (h Reads) WithAskedRoutes(asked AskedRoutes) Reads {
	h.askedRoutes = asked
	return h
}

// stampAvailability marks each candidate with what the introductions module
// says about it.
//
// A route is matched on the whole key, so a direct ask being open never greys
// out the same colleague's indirect route — the index would have accepted that
// one, and greying it out would refuse an ask the server does not.
func stampAvailability(
	routes []crmcontracts.PersonGraphRouteCandidate,
	asked map[introductions.RouteKey]introductions.RouteState,
) {
	for i := range routes {
		key := introductions.RouteKey{
			Introducer: ids.From[ids.UserKind](ids.UUID(routes[i].ViaUserId)),
		}
		if through := routes[i].ThroughPersonId; through != nil {
			key.Through = ids.From[ids.PersonKind](ids.UUID(*through))
		}
		switch asked[key] {
		case introductions.RouteOpen:
			routes[i].Availability = crmcontracts.PersonGraphRouteAvailabilityAlreadyRequested
		case introductions.RouteRefused:
			routes[i].Availability = crmcontracts.PersonGraphRouteAvailabilityDeclined
		}
	}
}
