// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// Which morning a brief belongs to, and the rule that a rep has exactly one.
//
// Both halves are one concern: the day boundary decides which morning a run is
// for, and uq_brief_run_user_day makes "one per morning" a fact the database
// keeps rather than one two racing writers hope for. Reading them apart is how
// they drift — a boundary computed one way by the writer and another by the
// reader serves yesterday's ranking under today's date.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
)

// LocalDayAt is the calendar date the given instant falls on in the
// installation's reporting zone, and the local wall clock at that instant —
// the "which morning is this, and has it started" that the overnight pass and
// the on-open read must agree on.
//
// It is exported because the overnight job asks the same question before it
// decides who is due a brief. Two spellings of a day boundary is two answers
// about which morning a run belongs to, and the pair disagreeing is precisely
// how a rep gets a run dated to a morning she has not lived yet.
//
// The installation zone, not the rep's: app_user.timezone is display-only and
// InviteUser never writes it, so scheduling on it would give every invited
// seat a UTC morning. installation.timezone is validated as a real IANA zone
// at write time, which is what makes a date computed from it reproducible.
func LocalDayAt(ctx context.Context, tx pgx.Tx, now time.Time) (day time.Time, local time.Time, err error) {
	zone, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("brief: the installation timezone %q does not resolve: %w", zone, err)
	}
	local = now.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC), local, nil
}

// localDay is LocalDayAt for the callers in this package, which need only the
// date.
func localDay(ctx context.Context, tx pgx.Tx, now time.Time) (time.Time, error) {
	day, _, err := LocalDayAt(ctx, tx, now)
	return day, err
}

// insertRunIfDayFree writes the run unless this rep already has one for the
// same local day, reporting which of the two happened. The unique constraint
// is the arbiter, so two writers racing produce one run and no error.
func insertRunIfDayFree(ctx context.Context, tx pgx.Tx, run BriefRun) (bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO brief_run (
			id, user_id, generated_at, as_of, local_day, candidate_count,
			revenue_norm_minor, revenue_norm_currency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT ON CONSTRAINT uq_brief_run_user_day DO NOTHING`,
		run.ID, run.UserID, run.GeneratedAt, run.AsOf, run.LocalDay, run.CandidateCount,
		run.RevenueNormMinor, run.RevenueNormCurrency)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
