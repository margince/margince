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
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/network"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The deal page and Worklist share the card builder and its evidence clock.
func newDealStatusService(pool *pgxpool.Pool, now func() time.Time) *dealstatus.Service {
	return newCurrentAdviceService(pool, now).WithSeats(dealSeatReader(pool))
}

func newCurrentAdviceService(pool *pgxpool.Pool, now func() time.Time) *dealstatus.Service {
	db := InstallationDB(pool)
	return dealstatus.NewService(pool, deals.NewStore(db, DealsInstallation()),
		activities.NewStore(db), dealrooms.NewStore(db), now).
		WithSeatBatch(dealSeatBatchReader(contacts.NewStore(db))).WithMailCoverage(mailCaptureCoverageReader(db))
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
// rather than failing. A reader who may see the seats but not the contacts gets
// the seats unnamed, which is contacts.ContactNamesTx's own posture.
func dealSeatReader(pool *pgxpool.Pool) dealstatus.SeatReader {
	contactsStore := contacts.NewStore(InstallationDB(pool))
	return func(ctx context.Context, dealID ids.DealID, now time.Time) ([]dealstatus.Seat, error) {
		var seats []dealstatus.Seat
		err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			coverage, err := network.CoverageFor(ctx, tx, dealID, now)
			if err != nil {
				return err
			}
			names, err := seatNamesForCard(ctx, tx, contactsStore, coverage.Stakeholders)
			if err != nil {
				return err
			}
			attachable, err := seatsThisReaderMayFileAgainst(ctx, tx, coverage.Stakeholders)
			if err != nil {
				return err
			}
			seats = make([]dealstatus.Seat, 0, len(coverage.Stakeholders))
			for _, s := range coverage.Stakeholders {
				seats = append(seats, dealstatus.Seat{
					Role: s.Role, Name: names[s.ContactID], Engaged: s.Engaged,
					ContactID: s.ContactID, Attachable: attachable[s.ContactID],
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

// seatsThisReaderMayFileAgainst answers, for every seated contact at once,
// whether this reader may attach work to them.
//
// READING A CONTACT IS NOT PERMISSION TO FILE AGAINST THEM. A share marked
// read-only shows somebody a contact and does not let them add to that
// contact's record, and activity_link's own writer enforces exactly that
// (auth.EnsureAttachTarget). A card that offered a task linked to such a
// contact would render a button that fails only once it is pressed, which
// reads to whoever pressed it as the product being broken rather than as
// a permission they do not have. So the card asks first and offers the advice
// without the link where the answer is no.
//
// It renders the attach predicate rather than calling EnsureAttachTarget per
// seat: that probe takes a FOR SHARE lock for the writers it is built for, and
// a card read has nothing to lock against. An unbounded reader gets an empty
// clause, which means every seat is attachable — the same shortcut every other
// scope caller takes.
func seatsThisReaderMayFileAgainst(
	ctx context.Context, tx pgx.Tx, seats []deals.DealStakeholder,
) (map[ids.UUID]bool, error) {
	attachable := map[ids.UUID]bool{}
	if len(seats) == 0 {
		return attachable, nil
	}
	// The object grant, asked before any contact row is touched. A seat this
	// reader may not read is one they certainly may not file against, and
	// answering "none attachable" is the same posture seatNamesForCard takes
	// when it may not name them: the card says less rather than failing.
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return attachable, nil
		}
		return nil, err
	}
	seated := make([]ids.UUID, 0, len(seats))
	for _, s := range seats {
		seated = append(seated, s.ContactID)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	seatedPos := arg(seated)
	clause, err := auth.AttachClauseFor(ctx, "contact", "c", arg)
	if err != nil {
		// A reader RBAC cannot judge gets no links rather than an error: the
		// card still has advice to give, and the only thing withheld is a
		// convenience the reader may not have been entitled to anyway.
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return attachable, nil
		}
		return nil, err
	}
	if clause == "" {
		for _, id := range seated {
			attachable[id] = true
		}
		return attachable, nil
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(
		`SELECT c.id FROM contact c WHERE c.id = ANY($%d) AND c.archived_at IS NULL AND %s`,
		seatedPos, clause), args...)
	if err != nil {
		return nil, fmt.Errorf("read which seated contacts accept filed work: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan a seated contact that accepts filed work: %w", err)
		}
		attachable[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read which seated contacts accept filed work: %w", err)
	}
	return attachable, nil
}

// seatNamesForCard names the seats, or names none of them.
//
// The permission-denied arm is the same one the coverage handler takes: a
// reader holding deal:read without contact:read still gets the deal's shape —
// how many contacts carry it, in what roles — and simply no names. The card then
// writes "the champion" where it would have written a contact.
func seatNamesForCard(
	ctx context.Context, tx pgx.Tx, store *contacts.Store, seats []deals.DealStakeholder,
) (map[ids.UUID]string, error) {
	if len(seats) == 0 {
		return map[ids.UUID]string{}, nil
	}
	seated := make([]ids.ContactID, 0, len(seats))
	for _, s := range seats {
		seated = append(seated, ids.From[ids.ContactKind](s.ContactID))
	}
	names, err := store.ContactNamesTx(ctx, tx, seated)
	if err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return map[ids.UUID]string{}, nil
		}
		return nil, err
	}
	return names, nil
}
