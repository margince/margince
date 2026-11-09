// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package weekly

// A week frozen under the retired qualified forecast is read back whole. Its
// measure keeps its word, and its retired bars still add up to the landings.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAWeekFrozenUnderTheQualifiedForecastIsReadBackWhole(t *testing.T) {
	e := setupWeekly(t)
	e.engine = e.engine.WithForecast(oneHorizon(ids.NewV7(), ids.NewV7()))
	written, _, err := e.engine.AssembleFor(e.repCtx, weekClock)
	if err != nil {
		t.Fatalf("assembling the week: %v", err)
	}
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	excludedDeal, notCheckedDeal := ids.NewV7(), ids.NewV7()
	for _, sql := range []string{
		`UPDATE weekly_review_outlook SET forward_measure = 'qualified_likely' WHERE weekly_review_id = $1`,
		// The frozen 300 to 420 landing carried two retired bars; the bars the
		// stub froze sum to 120, so the retired ones net to nothing.
		`INSERT INTO weekly_review_movement (weekly_review_id, period_kind, bar, delta_minor)
		 VALUES ($1, 'quarter', 'excluded', -2500), ($1, 'quarter', 'not_checked', 2500)`,
	} {
		if _, err := owner.Exec(ctx, sql, written.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := owner.Exec(ctx, `INSERT INTO weekly_review_driver
		(weekly_review_id, period_kind, bar, deal_id, deal_label, delta_minor)
		VALUES ($1, 'quarter', 'excluded', $2, 'Harbour refit', -2500),
		       ($1, 'quarter', 'not_checked', $3, 'Quay cranes', 2500)`,
		written.ID, excludedDeal, notCheckedDeal); err != nil {
		t.Fatal(err)
	}

	read, err := e.engine.LatestReview(e.repCtx, &written.LocalWeekStart)
	if err != nil {
		t.Fatalf("reading the review back: %v", err)
	}
	got := read.Outlook[0]
	if got.ForwardMeasure != "qualified_likely" {
		t.Errorf("the frozen measure read back as %q, want qualified_likely", got.ForwardMeasure)
	}
	sum := got.Opening.LandingMinor
	var other *Movement
	for i, bar := range got.Movement {
		sum += bar.DeltaMinor
		if bar.Bar == BarOther {
			other = &got.Movement[i]
		}
	}
	if sum != got.Closing.LandingMinor {
		t.Errorf("opening plus the bars is %d, want the closing landing %d", sum, got.Closing.LandingMinor)
	}
	if other == nil || len(other.Drivers) != 2 {
		t.Fatalf("the retired bars' drivers were not kept under %q: %+v", BarOther, got.Movement)
	}
}

func TestATeamWeekFrozenUnderTheQualifiedForecastKeepsItsMeasure(t *testing.T) {
	e := setupWeekly(t)
	e.engine = e.engine.WithForecast(oneHorizon(ids.NewV7(), ids.NewV7()))
	members := []TeamMember{{UserID: e.Rep1, DisplayName: "Rep One"}}
	team, _, err := e.engine.AssembleTeamFor(e.repCtx, e.Team1, "Team One", members, weekClock)
	if err != nil {
		t.Fatalf("assembling the team week: %v", err)
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `UPDATE team_weekly_review_outlook
		SET forward_measure = 'qualified_commit_evidence' WHERE team_weekly_review_id = $1`, team.ID); err != nil {
		t.Fatal(err)
	}

	read, err := e.engine.LatestTeamReview(e.repCtx, e.Team1, &team.LocalWeekStart)
	if err != nil {
		t.Fatalf("reading the team week back: %v", err)
	}
	if len(read.Outlook) != 1 || read.Outlook[0].ForwardMeasure != "qualified_likely" {
		t.Fatalf("the team outlook read back as %+v, want the qualified_likely it was frozen under", read.Outlook)
	}
}
