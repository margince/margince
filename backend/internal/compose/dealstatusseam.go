// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Who is on a deal, as the status card reads it.
//
// The card and the coverage chips sit on one screen and used to answer this
// question from different places — the chips through compose/network's
// CoverageFor, the card not at all. This binds the card to the SAME assembler,
// so a deal's seats are read once and the two surfaces cannot disagree about
// who is on it or how many there are.
//
// Writing a second seat read in dealstatus would have been the shorter path
// and the wrong one: CoverageFor carries the stakeholder edge admission
// (auth.EdgeReadAdmitted) that turns a denial into a named omission, and a
// copy would have been the version without it.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/network"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// newDealStatusService builds the deal's status card service.
//
// ONE constructor, because two surfaces read it and a deal has one next step.
// The deal page draws the card; the worklist reads the move that card decided,
// so a queue row and the deal page name the same thing. Building a second
// service for the queue would have been the shorter path and the wrong one: the
// two would carry different seat readers and different lane bindings, and the
// row would then suggest a step the page does not.
//
// It performs nothing. The click goes through the verb the move names, and the
// only row it writes is its own per-reader cache entry.
func newDealStatusService(pool *pgxpool.Pool) *dealstatus.Service {
	db := InstallationDB(pool)
	return dealstatus.NewService(
		pool, deals.NewStore(db, DealsInstallation()),
		activities.NewStore(db), dealrooms.NewStore(db), time.Now,
	).WithSeats(dealSeatReader(pool))
}

// dealSeatReader reads the deal's seats with their roles and names.
//
// One transaction for the coverage and the names, the way the coverage handler
// reads them: a seat list and the names for it taken from two snapshots can
// name somebody who is no longer on the deal.
//
// A reader refused the stakeholder edge gets no seats, not an error —
// CoverageFor answers a denial with an empty stakeholder list and a named
// omission, and the card's contract with the reader is that it says less
// rather than failing. A reader who may see the seats but not the people gets
// the seats unnamed, which is people.PersonNamesTx's own posture.
func dealSeatReader(pool *pgxpool.Pool) dealstatus.SeatReader {
	peopleStore := people.NewStore(InstallationDB(pool))
	return func(ctx context.Context, dealID ids.DealID, now time.Time) ([]dealstatus.Seat, error) {
		var seats []dealstatus.Seat
		err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			coverage, err := network.CoverageFor(ctx, tx, dealID, now)
			if err != nil {
				return err
			}
			names, err := seatNamesForCard(ctx, tx, peopleStore, coverage)
			if err != nil {
				return err
			}
			seats = make([]dealstatus.Seat, 0, len(coverage.Stakeholders))
			for _, s := range coverage.Stakeholders {
				seats = append(seats, dealstatus.Seat{
					Role: s.Role, Name: names[s.PersonID], Engaged: s.Engaged,
				})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return seats, nil
	}
}

// seatNamesForCard names the seats, or names none of them.
//
// The permission-denied arm is the same one the coverage handler takes: a
// reader holding deal:read without person:read still gets the deal's shape —
// how many people carry it, in what roles — and simply no names. The card then
// writes "the champion" where it would have written a person.
func seatNamesForCard(
	ctx context.Context, tx pgx.Tx, store *people.Store, coverage network.DealCoverage,
) (map[ids.UUID]string, error) {
	if len(coverage.Stakeholders) == 0 {
		return map[ids.UUID]string{}, nil
	}
	seated := make([]ids.PersonID, 0, len(coverage.Stakeholders))
	for _, s := range coverage.Stakeholders {
		seated = append(seated, ids.From[ids.PersonKind](s.PersonID))
	}
	names, err := store.PersonNamesTx(ctx, tx, seated)
	if err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return map[ids.UUID]string{}, nil
		}
		return nil, err
	}
	return names, nil
}
