// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

// Which doors are already taken, without saying who took them.
//
// The Network tab offers a route per colleague and the server refuses a second
// open ask on one. Until the tab can see that refusal coming it offers a door
// that is not there: the rep writes the reason, writes the note, presses send,
// and learns from a 409 what the page could have said before they started.
//
// So this is a read the tab makes to grey a route out. It answers with a
// STATUS and nothing else — no requester, no reason, no date. That distinction
// is the privacy of the thing: "Lena has already been asked about this
// contact" is a fact the rep needs in order not to waste the ask, while "Sofia
// asked her, and here is what Sofia wrote" is between Sofia and Lena.
// ForPerson serves the second question and stays scoped to the parties; this
// one serves the first, and is deliberately blind.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RouteKey identifies one route the way the duplicate-guard index does.
//
// A route is a colleague AND the intermediary they would go through, not the
// colleague alone: asking Lena directly and asking Lena through Marco are two
// different favours, and the index refuses them independently. Keying this
// read on anything coarser would grey out a route the server would have
// accepted.
type RouteKey struct {
	Introducer ids.UserID
	// Through is the contact the route passes through, ZERO for a direct one —
	// the same collapse the index makes with COALESCE onto the zero uuid.
	//
	// A value and not a pointer: a struct with a pointer field is compared by
	// pointer identity, so two keys naming the same intermediary would be
	// different map keys and every indirect route would read as unasked.
	Through ids.PersonID
}

// RouteState is what the tab may know about a route it did not ask about.
type RouteState string

const (
	// RouteOpen means an ask on this exact route is live: the duplicate guard
	// would refuse a second one.
	RouteOpen RouteState = "open"
	// RouteRefused means this colleague answered no to this contact before.
	// Not a bar — the rule is one OPEN ask, and a refusal is closed — but the
	// tab says so rather than letting a rep re-ask into the same no.
	RouteRefused RouteState = "refused"
)

// RouteStates reports which routes to this contact are already spoken for.
//
// Keyed by route, valued by state, and absent means nothing is known against
// it. Every ask about this contact is counted whoever made it, because the
// duplicate guard counts them all too — a read scoped to the caller's own asks
// would tell one rep the door is open while the index holds it shut.
//
// The compensating limit is the payload: a state, never a name. See the file
// header for why the two reads differ.
func (s *Store) RouteStates(
	ctx context.Context, personID ids.PersonID,
) (map[RouteKey]RouteState, error) {
	if err := auth.Require(ctx, "introduction", principal.ActionRead); err != nil {
		return nil, err
	}
	// A caller with no seat has no tab to render. Refusing here keeps this
	// blind read out of an agent's reach, which is the trade that lets it be
	// blind at all.
	if actor, ok := principal.Actor(ctx); !ok || actor.UserID.IsZero() {
		return nil, apperrors.ErrPermissionDenied
	}

	out := map[RouteKey]RouteState{}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The contact gate, on the read that discloses about them. Without it
		// a rep could probe which of their colleagues have been asked about a
		// person they may not open.
		if err := auth.EnsureVisibleLive(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT introducer_user_id, through_person_id, status
			  FROM intro_request
			 WHERE person_id = $1 AND archived_at IS NULL
			   AND status = ANY($2)`, personID, statesWorthReporting())
		if err != nil {
			return fmt.Errorf("introductions: reading which routes are taken: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var key RouteKey
			var status Status
			// The column is nullable and the key is not: a direct route stores
			// NULL and keys on the zero id, which is the collapse the guard
			// index spells as COALESCE.
			var through *ids.PersonID
			if err := rows.Scan(&key.Introducer, &through, &status); err != nil {
				return fmt.Errorf("introductions: reading a route's state: %w", err)
			}
			if through != nil {
				key.Through = *through
			}
			// An open ask outranks an old refusal on the same route: the rep
			// cannot act on either, and the live one is the truer sentence.
			if Open(status) {
				out[key] = RouteOpen
				continue
			}
			if _, taken := out[key]; !taken {
				out[key] = RouteRefused
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// statesWorthReporting is every status this read has a sentence for, derived
// from the lifecycle rather than spelled again.
//
// Hand-listing them here would make a fifth copy of the open set, and the way
// that fails is silent: a new open status would be missing from this list, the
// tab would offer the route, and the index would refuse the ask. Open() is
// already held against the duplicate-guard index, so deriving from it inherits
// that check.
// Held by: TestRouteStatesReportsEveryOpenStatus (routestate_test.go)
func statesWorthReporting() []string {
	out := []string{string(StatusDeclined)}
	for _, s := range everyStatus() {
		if Open(s) {
			out = append(out, string(s))
		}
	}
	return out
}
