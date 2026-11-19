// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The ONE definition of "an engaged stakeholder on this deal".
//
// It was already spelled inside the deal-health engine, where the composite
// reads it. The coverage and risk surfaces (ADR-0078) need the same answer,
// and a second spelling would let two screens disagree about whether a deal is
// single-threaded — which is precisely the flag reporting.md requires to
// reconcile across every surface that shows it (REPORT-PARAM-1).
//
// Engaged means a REAL two-way exchange in the window: both an inbound and an
// outbound qualifying interaction. A one-way broadcast target is not engaged,
// however many messages we sent them, and a deal threaded only through contacts
// who never replied is exactly the deal this flag exists to catch.

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EngagementWindowDays is the window the engagement test looks back over.
const EngagementWindowDays = healthEngagementWindowDays

// predicateAlways is a WHERE fragment that admits every row.
//
// It is what an absent clause becomes on the way into a statement: auth answers
// "" for a caller bounded by nothing and a nil filter has nothing to say, and an
// empty string interpolated into a WHERE is a syntax error rather than "no
// restriction". Named because three statements in this package make that
// substitution and the SQL they build must agree about it.
const predicateAlways = "true"

// edgeBound resolves the edge's read admission and returns the clause bounding
// WHICH edges, admitting every edge for a caller bounded by nothing.
//
// A function rather than two copies of an `if clause == ""`: both reads in this
// file take the same admission, so they take it through the same three lines.
func edgeBound(ctx context.Context, alias string, arg func(any) int) (string, error) {
	clause, err := auth.EdgeReadScope(ctx, alias, arg)
	if err != nil {
		return "", err
	}
	if clause == "" {
		return predicateAlways, nil
	}
	return clause, nil
}

// EngagedStakeholders lists the deal's live stakeholders who have had a
// two-way exchange inside the window, in deterministic id order.
//
// It takes the caller's transaction rather than opening its own: the callers
// are assembling a wider picture (a coverage payload, a risk scan) and must
// read one instant, not one per question.
//
// A seat is an EDGE, so this carries the edge's own admission: knowing a deal
// does not license learning who sits on it, and the endpoint grants do not
// cover the pair. The gate resolves before the statement — a caller refused it
// gets apperrors.ErrPermissionDenied and no rows are ever read, which is what
// lets CoverageFor name the omission instead of reporting an uncovered deal.
func EngagedStakeholders(ctx context.Context, tx pgx.Tx, dealID ids.DealID, now time.Time) ([]ids.UUID, error) {
	engaged, err := engagedAmong(ctx, tx, []ids.UUID{dealID.UUID}, now)
	if err != nil {
		return nil, err
	}
	out := make([]ids.UUID, 0, len(engaged))
	for pair := range engaged {
		out = append(out, pair.contact)
	}
	slices.SortFunc(out, func(a, b ids.UUID) int { return bytes.Compare(a[:], b[:]) })
	return out, nil
}

// DealStakeholder is one seat on a deal: who, in what role, and whether they
// are engaged.
type DealStakeholder struct {
	ContactID ids.UUID
	Role      string
	Engaged   bool
}

// Stakeholders lists every live seat on the deal with its role, marking the
// engaged ones — the shape a coverage view needs, where the unengaged seats
// are the finding rather than noise to filter out.
func Stakeholders(ctx context.Context, tx pgx.Tx, dealID ids.DealID, now time.Time) ([]DealStakeholder, error) {
	engaged, err := EngagedStakeholders(ctx, tx, dealID, now)
	if err != nil {
		return nil, err
	}
	isEngaged := make(map[ids.UUID]bool, len(engaged))
	for _, id := range engaged {
		isEngaged[id] = true
	}
	seats, err := StakeholderSeats(ctx, tx, []ids.UUID{dealID.UUID})
	if err != nil {
		return nil, err
	}
	out := seats[dealID.UUID]
	for i := range out {
		out[i].Engaged = isEngaged[out[i].ContactID]
	}
	return out, nil
}
