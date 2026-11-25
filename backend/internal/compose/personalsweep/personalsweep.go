// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package personalsweep runs one seat's pass of the personal-mail sweep and
// writes the receipt of what it did. The purger in the compose root supplies the
// destroy step, so this package never touches the purge cascade itself.
package personalsweep

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/purgerun"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Sweeper is the seat pass over one database.
type Sweeper struct {
	Pool *pgxpool.Pool
	// Batch bounds how many destroyable messages one pass selects per seat.
	Batch int
	// Floor resolves the statutory shield inside the caller's transaction.
	Floor func(ctx context.Context, tx pgx.Tx) (capture.StatutoryFloor, error)
	// Destroy purges a selection and returns how many messages it destroyed.
	Destroy func(ctx context.Context, subject capture.PurgeSubject, seat ids.UUID, windows capture.PersonalPurgeWindows) (int, error)
}

// Seat destroys one seat's due personal mail and writes the receipt for the
// pass. A pass that destroyed nothing writes none, so a seat whose due mail the
// shields keep does not add an audit row per tick. It returns how many messages
// it destroyed.
//
// The kept numbers are counted over every due message of the seat, apart from
// the batch the destroy reached.
func (s Sweeper) Seat(ctx context.Context, seat ids.UUID, windows capture.PersonalPurgeWindows) (int, error) {
	destroyed := 0
	err := purgerun.Hold(ctx, s.Pool, purgerun.Seat(seat), func(ctx context.Context, run ids.UUID) error {
		var err error
		destroyed, err = s.runSeat(ctx, run, seat, windows)
		return err
	})
	return destroyed, err
}

// runSeat is one run of the seat's pass, under the seat's lock.
func (s Sweeper) runSeat(ctx context.Context, run, seat ids.UUID, windows capture.PersonalPurgeWindows) (int, error) {
	var subject capture.PurgeSubject
	var kept capture.KeptCounts
	if err := database.WithWorkspaceTx(ctx, s.Pool, func(tx pgx.Tx) error {
		floor, err := s.Floor(ctx, tx)
		if err != nil {
			return err
		}
		if subject, err = capture.SelectPersonalPurgeTx(ctx, tx, seat, windows, floor, s.Batch); err != nil {
			return err
		}
		kept, err = capture.CountPersonalKeptTx(ctx, tx, seat, windows, floor)
		return err
	}); err != nil {
		return 0, fmt.Errorf("verdict: selecting personal mail for one seat: %w", err)
	}
	// No contacts are anonymised here. A `personal` verdict creates no
	// counterparty record, so there is nothing this sweep could match. A
	// selector that looked for one could find a contact made for another reason.
	if _, err := s.Destroy(ctx, subject, seat, windows); err != nil {
		return 0, fmt.Errorf("verdict: destroying personal mail: %w", err)
	}
	// Each destruction commits on its own, so the receipt cannot join them. A
	// failed write loses no evidence: the receipt sweep writes it from the rows
	// of this run. A run that touched nothing writes none.
	tally, err := purgerun.WriteReceipt(ctx, s.Pool, purgerun.Seat(seat), run, func(t purgerun.Tally) map[string]any {
		if t.Destroyed == 0 && t.Released == 0 {
			return nil
		}
		return map[string]any{
			"destroyed":                     t.Destroyed,
			"skipped":                       kept.UnderStatute + kept.UnderRequest + kept.UnderUndeterminedFloor,
			"kept_held":                     kept.Held,
			"kept_under_statute":            kept.UnderStatute,
			"kept_under_request":            kept.UnderRequest,
			"kept_under_undetermined_floor": kept.UnderUndeterminedFloor,
		}
	})
	if err != nil {
		return tally.Destroyed, fmt.Errorf("verdict: recording the personal sweep receipt: %w", err)
	}
	return tally.Destroyed, nil
}
