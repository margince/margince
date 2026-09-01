// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Network tab asks the introductions module which doors are taken.
//
// Two modules, and neither may import the other, so the edge is made here.
// Without it the tab offers every route as free and the rep discovers the
// duplicate guard by being refused after writing the whole ask.
//
// The translation is the whole of this file: introductions answers in its own
// vocabulary, network compares against its own, and neither has to know the
// other's spelling. Both are string constants and the pairing is checked, so
// a rename on either side fails a test rather than silently unblocking every
// route.

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/network"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// askedRoutesSeam adapts the introductions reader to what network asks for.
type askedRoutesSeam struct {
	store *introductions.Store
}

// newAskedRoutesSeam binds the reader that knows which routes are spoken for.
func newAskedRoutesSeam(store *introductions.Store) askedRoutesSeam {
	return askedRoutesSeam{store: store}
}

// RouteStates translates both the key and the verdict across the seam.
//
// Held by: TestTheRouteSeamTranslatesEveryState (introroutesseam_test.go),
// which fails if either side gains a state this mapping drops — a dropped
// state reads as "no answer", which is exactly "the route is free".
func (s askedRoutesSeam) RouteStates(
	ctx context.Context, personID ids.PersonID,
) (map[network.RouteKey]network.RouteState, error) {
	taken, err := s.store.RouteStates(ctx, personID)
	if err != nil {
		return nil, err
	}
	out := make(map[network.RouteKey]network.RouteState, len(taken))
	for key, state := range taken {
		translated, ok := networkRouteState(state)
		if !ok {
			// A state this seam has no word for must not read as "free". The
			// introductions module reported it precisely because the route is
			// spoken for, so the safe reading is the blocking one: the rep is
			// told the door is taken and the server agrees when they try.
			translated = network.RouteOpen
		}
		out[network.RouteKey{Introducer: key.Introducer, Through: key.Through}] = translated
	}
	return out, nil
}

// networkRouteState is the one place the two vocabularies are paired.
func networkRouteState(state introductions.RouteState) (network.RouteState, bool) {
	switch state {
	case introductions.RouteOpen:
		return network.RouteOpen, true
	case introductions.RouteRefused:
		return network.RouteRefused, true
	default:
		return "", false
	}
}
