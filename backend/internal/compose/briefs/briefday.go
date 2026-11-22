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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/dayrule"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// LocalDayAt is the calendar date at now for one person's brief, and their wall
// clock at that instant. The overnight pass and the on-open read both ask it.
// A brief is the person's own list, so the day follows their zone.
func LocalDayAt(ctx context.Context, tx pgx.Tx, user ids.UUID, now time.Time) (day time.Time, local time.Time, err error) {
	return dayrule.DayAt(ctx, tx, occurredday.Personal, user, now)
}

// OvernightFrom is 05:00 on the person's own morning, the earliest the overnight
// pass assembles their run unless their delivery window opens it sooner.
func OvernightFrom(ctx context.Context, pool *pgxpool.Pool, user ids.UUID, now time.Time) (time.Time, error) {
	var from time.Time
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		_, local, err := LocalDayAt(ctx, tx, user, now)
		from = time.Date(local.Year(), local.Month(), local.Day(), 5, 0, 0, 0, local.Location())
		return err
	})
	return from, err
}

// localDay is LocalDayAt for the callers in this package, which need only the
// date.
func localDay(ctx context.Context, tx pgx.Tx, user ids.UUID, now time.Time) (time.Time, error) {
	day, _, err := LocalDayAt(ctx, tx, user, now)
	return day, err
}

// insertRunIfDayFree writes the run unless this rep already has one for the
// same local day, reporting which of the two happened. The unique constraint
// is the arbiter, so two writers racing produce one run and no error.
func insertRunIfDayFree(ctx context.Context, tx pgx.Tx, run BriefRun) (bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO brief_run (
			id, user_id, generated_at, as_of, local_day, candidate_count,
			revenue_norm_minor, revenue_norm_currency, factors_omitted)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT ON CONSTRAINT uq_brief_run_user_day DO NOTHING`,
		run.ID, run.UserID, run.GeneratedAt, run.AsOf, run.LocalDay, run.CandidateCount,
		run.RevenueNormMinor, run.RevenueNormCurrency, run.FactorsOmitted)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
