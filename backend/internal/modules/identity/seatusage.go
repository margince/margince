// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// How many seats the installation is using, for the entitlement surface. The
// LICENSE half of that surface is not here — what a license grants is resolved
// from the deployment file and the bundled validation module, which identity
// knows nothing about — so the composition root pairs this count with the
// posture (ADR-0054: a cross-layer edge is injected, never imported).

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// licenseObject is the RBAC object gating the entitlement read. Admin/ops only,
// read included: a seat meter is the installation's commercial standing, and a
// rep reads their own seat elsewhere (UC-ADMIN-03 F1).
const licenseObject = "license"

// seatUsageObject gates the seat COUNT on its own, apart from the entitlement it
// is normally read beside. The two answer different questions to different
// readers: how many seats are in use is capacity, which a manager plans against,
// while what the license grants and what it cost is the installation's
// commercial standing. One object for both meant a role could not be shown the
// first without also being handed the second.
const seatUsageObject = "seat_usage"

// SeatUsageStore counts the seats an entitlement is measured against.
type SeatUsageStore struct {
	db *database.DB
}

// NewSeatUsage builds the store on a handle already bound to the workspace it
// serves.
func NewSeatUsage(db *database.DB) *SeatUsageStore { return &SeatUsageStore{db: db} }

// FullSeatsInUse counts the full seats this installation is using: every
// non-deactivated one held by a person.
//
// Three decisions the count makes, each of which the meter would be wrong
// without:
//
// READ SEATS ARE NOT COUNTED. They are unlimited and never metered — that is the
// whole of A62/ADR-0047, and the reason a workspace can hand out viewers freely.
//
// A SUSPENDED SEAT IS NOT COUNTED. Suspension is how an admin stops somebody
// acting without erasing them, so counting one would bill for access the
// installation has already withdrawn.
//
// AN AGENT SEAT IS NOT COUNTED, because LICENSE says so: a Seat is "a single,
// identified natural person", and automated agents that act under the authority
// of a counted Seat explicitly do not count. That document is what a customer
// relies on and what this meter is measured against, so the meter follows it —
// counting agents would cap an installation for something its licence gives
// away, and the first customer to read both carefully would find it.
//
// It does not let an installation act without limit through agents, which is the
// reading the exclusion invites: the licence admits an agent only where it is
// attributable to a counted Seat, so an installation with no seats has no
// authority for one to act under.
//
// `is_agent` is the whole of it. The licence also excludes service accounts, and
// app_user carries no second marker for one — the only service account in this
// product is Google's Pub/Sub signer, which is an external identity and holds no
// row here. A second exclusion would be inventing a distinction the schema does
// not make.
//
// This is deliberately NOT the spec's "active" definition (signed in within 30
// days, UC-ADMIN-03 precondition 3): the two meters therefore disagree, which is
// recorded on issue #1190 for reconciliation rather than resolved here.
func (s *SeatUsageStore) FullSeatsInUse(ctx context.Context) (int, error) {
	if err := auth.Require(ctx, licenseObject, principal.ActionRead); err != nil {
		return 0, err
	}
	return s.countFullSeats(ctx)
}

// SeatsInUse answers the same count for a reader holding `seat_usage` rather
// than `license`: the capacity question without the commercial one.
//
// It runs countFullSeats rather than a statement of its own, and that is the
// point of splitting the GRANT instead of writing a second store. Two counts of
// one installation's seats would drift, and the one that drifted would be the
// one nobody was refused a seat by — a meter disagreeing with the ceiling it
// measures. There is one meter; this is a second door onto it, and the doors
// differ only in which grant opens them.
func (s *SeatUsageStore) SeatsInUse(ctx context.Context) (int, error) {
	if err := auth.Require(ctx, seatUsageObject, principal.ActionRead); err != nil {
		return 0, err
	}
	return s.countFullSeats(ctx)
}

// countFullSeats runs the meter. It carries no gate of its own, which is why it
// is unexported: both entry points above check one first, and a caller reaching
// the count without checking would be the defect this shape prevents.
func (s *SeatUsageStore) countFullSeats(ctx context.Context) (int, error) {
	var used int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, fullSeatsInUseQuery).Scan(&used)
	})
	if err != nil {
		return 0, fmt.Errorf("identity: counting full seats in use: %w", err)
	}
	return used, nil
}

// fullSeatsInUseQuery is the ONE count the license is measured against: the
// meter an admin reads on the entitlement screen and the ceiling that refuses
// them a seat run this same statement. A meter nobody is held to and a ceiling
// nobody can see are the same defect from two sides, and the only way the two
// cannot drift is that there is one of them.
//
// The predicate is the three decisions above: full seats only, no agents, and
// neither a suspended nor a deactivated seat — both are access the installation
// has already withdrawn, and an admin who suspended somebody to free a seat has
// to actually get it back.
//
// `NOT is_agent` is the licence's own exclusion and not a filter somebody added:
// a Seat is a natural person there, so an agent row is not one of these however
// much estate it touches. Removing it would meter what the licence gives away.
//
// It names the statuses that do NOT count rather than the one that does. A seat
// exists until the installation withdraws it, so a status added later should
// count until somebody decides otherwise: the wrong way round would silently
// stop metering a state nobody had thought about, and an installation would
// issue seats its license never granted.
const fullSeatsInUseQuery = `SELECT count(*) FROM app_user
	 WHERE seat_type = 'full' AND NOT is_agent
	   AND status NOT IN ('suspended', 'deactivated')`
