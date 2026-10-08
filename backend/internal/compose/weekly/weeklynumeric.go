// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// NumericResult carries the canonical measures that replace legacy Weekly tallies.
type NumericResult struct {
	Summary                 crmcontracts.WeeklyNumericSummary
	Won, Held, WithNextStep int
}

// NumericWeek lets Weekly consume reporting calculations without importing its module.
type NumericWeek interface {
	Measure(context.Context, pgx.Tx, string, ids.UUID, time.Time, time.Time, time.Time) (NumericResult, error)
}

// WithNumeric binds Weekly to the shared metric engine when reporting is enabled.
func (e *Engine) WithNumeric(source NumericWeek) *Engine {
	e.numeric = source
	return e
}

func (e *Engine) measureNumeric(ctx context.Context, tx pgx.Tx, review *Review, start, end, now time.Time) error {
	if e.numeric == nil {
		return nil
	}
	numeric, err := e.numeric.Measure(ctx, tx, "owner", review.UserID, start, end, now)
	if err != nil {
		return err
	}
	review.NumericSummary = &numeric.Summary
	review.Counts.DealsWon = numeric.Won
	review.Counts.MeetingsHeld = numeric.Held
	review.Counts.MeetingsWithNextStep = numeric.WithNextStep
	applyNumericMoney(&review.Money, numeric.Summary)
	return nil
}

func (e *Engine) measureTeamNumeric(
	ctx context.Context, tx pgx.Tx, review *TeamReview, members []TeamMember, now time.Time,
) error {
	if e.numeric == nil {
		return nil
	}
	start, end, err := localWeekWindow(ctx, tx, review.LocalWeekStart)
	if err != nil {
		return err
	}
	numeric, err := e.numeric.Measure(ctx, tx, "team", review.TeamID, start, end, now)
	if err != nil {
		return err
	}
	owners := make([]ids.UUID, 0, len(members))
	for _, member := range members {
		owners = append(owners, member.UserID)
	}
	// The team's figures sum its members' weeks, so a family is measured from
	// the first record any member's source holds.
	numeric.Summary.FigureCoverage, err = figureCoverageOf(ctx, tx, owners, start, end, e.plan != nil)
	if err != nil {
		return err
	}
	review.NumericSummary = &numeric.Summary
	review.Counts.DealsWon = numeric.Won
	review.Counts.MeetingsHeld = numeric.Held
	review.Counts.MeetingsWithNextStep = numeric.WithNextStep
	applyNumericMoney(&review.Money, numeric.Summary)
	return nil
}

func applyNumericMoney(money *Money, summary crmcontracts.WeeklyNumericSummary) {
	if summary.WonMinor == nil || money.Currency != summary.Currency {
		money.Known = false
		return
	}
	money.WonMinor = *summary.WonMinor
}

func (e *Engine) withMeasurementSnapshot(ctx context.Context, read func(pgx.Tx) error) error {
	return database.WithWorkspaceSnapshot(ctx, e.pool, func(snapshot context.Context) error {
		return database.WithWorkspaceTx(snapshot, e.pool, read)
	})
}
