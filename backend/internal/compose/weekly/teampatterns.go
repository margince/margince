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
func (said teamCopy) patternsFor(counts Counts, help int, deals *DealBlock) []Pattern {
	var out []Pattern
	add := func(kind, label string) { out = append(out, Pattern{Kind: kind, Label: label}) }
	if help > 0 {
		add(FocusHelpRequested, said.countPhrase(patternWords.HelpOne, patternWords.HelpMany, help))
	}
	if counts.LeadsBreached > 0 {
		add(FocusLeadsBreached, said.countPhrase(patternWords.BreachOne, patternWords.BreachMany, counts.LeadsBreached))
	}
	if counts.CommitmentsDue > counts.CommitmentsKept {
		add(FocusCommitmentsMissed, fmt.Sprintf(said.Say(patternWords.Kept),
			counts.CommitmentsKept, counts.CommitmentsDue))
	}
	if deals != nil {
		out = append(out, said.dealPatterns(*deals)...)
	}
	// Once, whichever of the two reasons holds: two rows saying "copy this
	// rep" are one finding spelled twice.
	switch {
	case counts.DealsWon > 0:
		add(FocusStrongWeek, said.countPhrase(patternWords.WonOne, patternWords.WonMany, counts.DealsWon))
	case counts.CommitmentsDue > 0 && counts.CommitmentsDue == counts.CommitmentsKept:
		add(FocusStrongWeek, fmt.Sprintf(said.Say(patternWords.KeptAll), counts.CommitmentsDue))
	}
	if counts.MeetingsHeld > counts.MeetingsWithNextStep {
		add(FocusMeetingsWithoutNextStep, said.countPhrase(patternWords.MeetingOne, patternWords.MeetingMany, counts.MeetingsHeld-counts.MeetingsWithNextStep))
	}
	if len(out) == 0 {
		add(FocusQuietWeek, said.Say(patternWords.Quiet))
	}
	return out
}

// dealPatterns are the four rules over the frozen deal scorecard.
func (said teamCopy) dealPatterns(d DealBlock) []Pattern {
	var out []Pattern
	if d.ForecastDown > 0 {
		out = append(out, Pattern{FocusForecastDowngrades, said.countPhrase(patternWords.ForecastOne, patternWords.ForecastMany, d.ForecastDown)})
	}
	if d.Regressions > 0 {
		out = append(out, Pattern{FocusStageRegressions, said.countPhrase(patternWords.RegressedOne, patternWords.RegressedMany, d.Regressions)})
	}
	// A nil count is a scorecard frozen before it was measured, which says
	// nothing about the rep's close dates.
	if p := d.CloseDateProvisional; p != nil && d.Open >= coverageRuleMinimumOpenDeals &&
		*p*basisPointsWhole > provisionalCloseDatesCeilingBp*d.Open {
		out = append(out, Pattern{FocusCloseDatesProvisional, fmt.Sprintf(
			said.Say(patternWords.Provisional), *p, d.Open)})
	}
	if d.Open >= coverageRuleMinimumOpenDeals &&
		d.WithNextStep*basisPointsWhole < nextStepCoverageFloorBp*d.Open {
		out = append(out, Pattern{FocusNoNextStepCoverage, fmt.Sprintf(
			said.Say(patternWords.NextStep), d.WithNextStep, d.Open)})
	}
	return out
}

// focusFor is the one thing to raise first: the head of the pattern list.
func (said teamCopy) focusFor(counts Counts, help int, deals *DealBlock) (kind, label string) {
	first := said.patternsFor(counts, help, deals)[0]
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
