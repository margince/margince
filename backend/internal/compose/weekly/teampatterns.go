// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The rules that name what a lead should raise with a rep. They are tried, and
// the list is sorted, in the order below.
//
// The order is what a lead should raise first, not severity in the abstract. A
// rep who asked for help has already said what they need. Walking past that to
// raise a metric teaches them not to ask.
const (
	// They asked. Nothing outranks a request already made.
	FocusHelpRequested = "help_requested"
	// A customer waited past the response target, even if answered later.
	FocusLeadsBreached = "leads_breached"
	// They planned a week and did not keep it.
	FocusCommitmentsMissed = "commitments_missed"
	// Retired from the writer and kept in the vocabulary. Snapshots frozen
	// before the four deal rules below carry it, and no migration rewrites them.
	FocusDealsAtRisk = "deals_at_risk"
	// A deal's forecast category went down inside the week.
	FocusForecastDowngrades = "forecast_downgrades"
	// A deal moved to an earlier stage inside the week.
	FocusStageRegressions = "stage_regressions"
	// Too many open deals closed the week on a provisional close date.
	FocusCloseDatesProvisional = "close_dates_provisional"
	// Too few open deals closed the week with a next step.
	FocusNoNextStepCoverage = "no_next_step_coverage"
	// Nothing to fix, and something to copy. Without this a healthy rep
	// produces no row at all, and the page would read as a team where only
	// the troubled contacts exist.
	FocusStrongWeek = "strong_week"
	// Meetings have no explicit follow-up recorded.
	FocusMeetingsWithoutNextStep = "meetings_without_next_step"
	// Nothing to fix and nothing that stood out. Said plainly rather than
	// dressed as either a problem or a triumph.
	FocusQuietWeek = "quiet_week"
)

// The bars the two ratio rules hold a rep's open deals to.
const (
	// Below this many open deals a ratio is an anecdote. One deal of two
	// without a next step is half the book and says nothing about habit.
	coverageRuleMinimumOpenDeals = 3
	// Under half of the open deals with a next step fires the rule.
	nextStepCoverageFloorBp = 5000
	// Over three in ten open deals on a provisional close date fires it.
	provisionalCloseDatesCeilingBp = 3000
	basisPointsWhole               = 10_000 // money-scale-exempt: basis points, not a currency scale
)

// Pattern is one named thing a lead should raise with a rep.
type Pattern struct {
	Kind  string
	Label string
}

// patternsFor names every rule that fires for one rep's frozen week, in the
// order a lead raises them. Never empty: a week where nothing fired is a
// quiet week, said as one.
//
// The labels are composed here rather than by a model. Each states a stored
// figure and nothing else, so it says nothing the snapshot does not hold.
func patternsFor(counts Counts, help int, deals *DealBlock) []Pattern {
	var out []Pattern
	add := func(kind, label string) { out = append(out, Pattern{Kind: kind, Label: label}) }
	if help > 0 {
		add(FocusHelpRequested, fmt.Sprintf("Asked for help on %s", plural(help, "commitment")))
	}
	if counts.LeadsBreached > 0 {
		add(FocusLeadsBreached, fmt.Sprintf("%s went past the response target",
			plural(counts.LeadsBreached, "lead")))
	}
	if counts.CommitmentsDue > counts.CommitmentsKept {
		add(FocusCommitmentsMissed, fmt.Sprintf("Kept %d of %d commitments",
			counts.CommitmentsKept, counts.CommitmentsDue))
	}
	if deals != nil {
		out = append(out, dealPatterns(*deals)...)
	}
	// Once, whichever of the two reasons holds: two rows saying "copy this
	// rep" are one finding spelled twice.
	switch {
	case counts.DealsWon > 0:
		add(FocusStrongWeek, fmt.Sprintf("Won %s — worth asking how", plural(counts.DealsWon, "deal")))
	case counts.CommitmentsDue > 0 && counts.CommitmentsDue == counts.CommitmentsKept:
		add(FocusStrongWeek, fmt.Sprintf("Kept every commitment (%d)", counts.CommitmentsDue))
	}
	if counts.MeetingsHeld > counts.MeetingsWithNextStep {
		add(FocusMeetingsWithoutNextStep, fmt.Sprintf("%s have no linked follow-up recorded",
			plural(counts.MeetingsHeld-counts.MeetingsWithNextStep, "meeting")))
	}
	if len(out) == 0 {
		add(FocusQuietWeek, "No priority indicated by the recorded metrics")
	}
	return out
}

// dealPatterns are the four rules over the frozen deal scorecard.
func dealPatterns(d DealBlock) []Pattern {
	var out []Pattern
	if d.ForecastDown > 0 {
		out = append(out, Pattern{FocusForecastDowngrades, fmt.Sprintf(
			"Forecast downgraded on %s — review the recovery plan", plural(d.ForecastDown, "deal"))})
	}
	if d.Regressions > 0 {
		out = append(out, Pattern{FocusStageRegressions, fmt.Sprintf(
			"%s moved backwards — review the recovery plan", plural(d.Regressions, "deal"))})
	}
	// A nil count is a scorecard frozen before it was measured, which says
	// nothing about the rep's close dates.
	if p := d.CloseDateProvisional; p != nil && d.Open >= coverageRuleMinimumOpenDeals &&
		*p*basisPointsWhole > provisionalCloseDatesCeilingBp*d.Open {
		out = append(out, Pattern{FocusCloseDatesProvisional, fmt.Sprintf(
			"%d of %d open deals have a provisional close date — confirm the forecast", *p, d.Open)})
	}
	if d.Open >= coverageRuleMinimumOpenDeals &&
		d.WithNextStep*basisPointsWhole < nextStepCoverageFloorBp*d.Open {
		out = append(out, Pattern{FocusNoNextStepCoverage, fmt.Sprintf(
			"%d of %d open deals have a next step — agree an owner and date", d.WithNextStep, d.Open)})
	}
	return out
}

// focusFor is the one thing to raise first: the head of the pattern list.
func focusFor(counts Counts, help int, deals *DealBlock) (kind, label string) {
	first := patternsFor(counts, help, deals)[0]
	return first.Kind, first.Label
}

// memberDealBlock reads the rep's frozen deal scorecard for the week, or nil.
// A week with no scorecard supplies no deal finding rather than a healthy one.
func memberDealBlock(ctx context.Context, tx pgx.Tx, userID ids.UUID, week time.Time) (*DealBlock, error) {
	var d DealBlock
	err := tx.QueryRow(ctx, `SELECT coalesce(s.deal_forecast_down, 0), coalesce(s.deal_regressions, 0),
   coalesce(s.deal_open, 0), coalesce(s.deal_close_date_sound, 0), coalesce(s.deal_with_next_step, 0),
   s.deal_close_date_provisional
   FROM weekly_review r JOIN weekly_review_scorecard s ON s.weekly_review_id = r.id
   WHERE r.user_id = $1 AND r.local_week_start = $2 AND s.has_deal_block`, userID, week).
		Scan(&d.ForecastDown, &d.Regressions, &d.Open, &d.CloseDateSound, &d.WithNextStep,
			&d.CloseDateProvisional)
	if errors.Is(err, pgx.ErrNoRows) {
		//nolint:nilnil // a week with no deal scorecard has no deal finding, which is not an error.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the member's frozen deal scorecard: %w", err)
	}
	return &d, nil
}

// plural renders a count with its noun, so a label reads as a sentence.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
