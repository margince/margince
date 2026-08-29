// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// §11 fixed-clock tables: every hygiene flag, the A6 tier policy with
// both CLOSE_DATE_AUTOAPPLY positions, and the replacement-date math.

import (
	"testing"
	"time"
)

// closeClock is noon UTC so the workspace-zone date is unambiguous.
var closeClock = time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

func datep(daysFromToday int) *time.Time {
	v := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC).AddDate(0, 0, daysFromToday)
	return &v
}

func tsp(daysAgo int) *time.Time {
	v := closeClock.AddDate(0, 0, -daysAgo)
	return &v
}

// activeDeal is a base fixture: open, touched 3 days ago, early stage,
// outside the forecast.
func activeDeal(expectedClose *time.Time) CloseDateInput {
	return CloseDateInput{
		Status:              "open",
		ExpectedClose:       expectedClose,
		CreatedAt:           closeClock.AddDate(0, 0, -120),
		LastActivityAt:      tsp(3),
		StageWinProbability: 20,
		RemainingOpenStages: 2,
	}
}

func hasFlag(h CloseDateHygiene, f CloseDateFlag) bool {
	for _, got := range h.Flags {
		if got == f {
			return true
		}
	}
	return false
}

func TestCloseDateAssessmentFlags(t *testing.T) {
	quiet := func(in CloseDateInput) CloseDateInput {
		in.LastActivityAt = tsp(90)
		return in
	}
	cases := []struct {
		name string
		in   CloseDateInput
		want []CloseDateFlag
	}{
		{"healthy future date", activeDeal(datep(45)), nil},
		{"closing today is fine (strict <)", func() CloseDateInput {
			in := activeDeal(datep(0))
			in.StageWinProbability = 60 // isolate overdue from the soon window
			return in
		}(), nil},
		{"past date is overdue", activeDeal(datep(-12)), []CloseDateFlag{CloseDateOverdue}},
		{"missing date", activeDeal(nil), []CloseDateFlag{CloseDateMissing}},
		{"early stage within the soon window", activeDeal(datep(7)), []CloseDateFlag{CloseDateUnrealisticSoon}},
		{"soon window is credible at 40%", func() CloseDateInput {
			in := activeDeal(datep(7))
			in.StageWinProbability = 40
			return in
		}(), nil},
		{"stalled deal with a near date", quiet(activeDeal(datep(45))), []CloseDateFlag{CloseDateUnrealisticStale}},
		{"stalled deal with a far date", quiet(activeDeal(datep(90))), nil},
		{"future wait suppresses stale but never overdue", func() CloseDateInput {
			in := quiet(activeDeal(datep(-5)))
			in.WaitUntil = datep(60)
			return in
		}(), []CloseDateFlag{CloseDateOverdue}},
		{"closed deals are never flagged", func() CloseDateInput {
			in := activeDeal(datep(-30))
			in.Status = "won"
			return in
		}(), nil},
	}
	for _, c := range cases {
		got := CloseDateAssessment(c.in, closeClock, time.UTC)
		if got.Flagged != (len(c.want) > 0) || len(got.Flags) != len(c.want) {
			t.Errorf("%s: flags = %v, want %v", c.name, got.Flags, c.want)
			continue
		}
		for _, f := range c.want {
			if !hasFlag(got, f) {
				t.Errorf("%s: flags = %v, missing %s", c.name, got.Flags, f)
			}
		}
		if !got.Flagged && (got.ProposedClose != nil || got.Action != "") {
			t.Errorf("%s: unflagged deal grew a correction: %+v", c.name, got)
		}
	}
}

func TestCloseDateActionTiers(t *testing.T) {
	commit := func(in CloseDateInput) CloseDateInput {
		in.InForecastCommit = true
		return in
	}
	cases := []struct {
		name            string
		in              CloseDateInput
		wantAction      CloseDateAction
		wantProvisional bool
		wantDowngrade   bool
	}{
		// §11 worked example: early-stage, clear-overdue, active, outside
		// the forecast → the agent rolls the date, final.
		{"clear overdue on an active low-stakes deal auto-applies",
			activeDeal(datep(-12)), CloseDateActionAutoApply, false, false},
		{"forecast-bearing overdue goes provisional",
			commit(activeDeal(datep(-12))), CloseDateActionProvisionalConfirm, true, false},
		{"late stage overdue goes provisional", func() CloseDateInput {
			in := activeDeal(datep(-12))
			in.StageWinProbability = 60
			return in
		}(), CloseDateActionProvisionalConfirm, true, false},
		{"missing date goes provisional",
			activeDeal(nil), CloseDateActionProvisionalConfirm, true, false},
		{"unrealistic-soon goes provisional",
			activeDeal(datep(5)), CloseDateActionProvisionalConfirm, true, false},
		// A paused deal still must not claim a past date (§11 edge case):
		// the wait suppresses quiet, not overdue — 🟡, never 🟢-silent.
		{"paused overdue commit deal goes provisional", func() CloseDateInput {
			in := commit(activeDeal(datep(-5)))
			in.LastActivityAt = tsp(90)
			in.WaitUntil = datep(60)
			return in
		}(), CloseDateActionProvisionalConfirm, true, false},
		// Even the otherwise-🟢 case: the customer said wait, so a
		// stage-velocity guess is never silently finalized on their deal.
		{"paused overdue low-stakes deal is never auto-finalized", func() CloseDateInput {
			in := activeDeal(datep(-5))
			in.WaitUntil = datep(60)
			return in
		}(), CloseDateActionProvisionalConfirm, true, false},
		{"quiet overdue deal downgrades with a provisional invariant date", func() CloseDateInput {
			in := activeDeal(datep(-12))
			in.LastActivityAt = tsp(90)
			return in
		}(), CloseDateActionDowngradeAndReview, true, true},
		// The zombie guard: a quiet deal with a future-but-stale date is
		// downgraded WITHOUT touching the date at all.
		{"quiet future-dated deal downgrades without a provisional date", func() CloseDateInput {
			in := activeDeal(datep(30))
			in.LastActivityAt = tsp(90)
			return in
		}(), CloseDateActionDowngradeAndReview, false, true},
	}
	for _, c := range cases {
		got := CloseDateAssessment(c.in, closeClock, time.UTC)
		if !got.Flagged {
			t.Errorf("%s: expected a flagged deal, got %+v", c.name, got)
			continue
		}
		if got.Action != c.wantAction || got.Provisional != c.wantProvisional || got.Downgrade != c.wantDowngrade {
			t.Errorf("%s: (action, provisional, downgrade) = (%s, %v, %v), want (%s, %v, %v)",
				c.name, got.Action, got.Provisional, got.Downgrade, c.wantAction, c.wantProvisional, c.wantDowngrade)
		}
	}
}

// CLOSE_DATE_AUTOAPPLY off reverts the 🟢 tier to 🟡 for the exact case
// that would otherwise auto-apply — and leaves the 🔻 tier alone.
func TestCloseDateAutoApplySwitchOffRoutesEverythingProvisional(t *testing.T) {
	in := activeDeal(datep(-12))
	if got := closeDateAction(closeDateFindings{overdue: true}, in, false); got != CloseDateActionProvisionalConfirm {
		t.Errorf("switch off: action = %s, want provisional_confirm", got)
	}
	if got := closeDateAction(closeDateFindings{stalled: true, overdue: true}, in, false); got != CloseDateActionDowngradeAndReview {
		t.Errorf("switch off must not rescue a quiet deal: action = %s", got)
	}
}

func TestProposedCloseDateUsesVelocityAndStageFloor(t *testing.T) {
	today := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		remaining int
		velocity  float64
		wantDays  int
	}{
		{"fallback velocity, two stages", 2, 0, 2 * CloseDateStageDays},
		{"observed velocity outranks the fallback", 2, 18, 36},
		{"at least one stage-worth even on the last stage", 0, 18, 18},
	}
	for _, c := range cases {
		got := proposedCloseDate(today, c.remaining, c.velocity)
		if want := today.AddDate(0, 0, c.wantDays); !got.Equal(want) {
			t.Errorf("%s: proposed = %s, want %s", c.name, got.Format(time.DateOnly), want.Format(time.DateOnly))
		}
	}
}

// Today buckets in the workspace zone (data-semantics §2 r4): late
// evening UTC is already tomorrow in Auckland, so a date that is "today"
// in UTC reads overdue there.
func TestCloseDateTodayBucketsInWorkspaceZone(t *testing.T) {
	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatal(err)
	}
	lateUTC := time.Date(2026, 7, 4, 23, 0, 0, 0, time.UTC) // July 5 in Auckland
	in := activeDeal(datep(0))                              // claims July 4
	in.StageWinProbability = 60                             // keep the soon window out of the picture
	if got := CloseDateAssessment(in, lateUTC, time.UTC); got.Flagged {
		t.Errorf("UTC workspace: July 4 at 23:00 UTC flagged %v, want clean", got.Flags)
	}
	got := CloseDateAssessment(in, lateUTC, auckland)
	if !hasFlag(got, CloseDateOverdue) {
		t.Errorf("Auckland workspace: flags = %v, want overdue", got.Flags)
	}
}

// TestOverdueIsAskedInTheInstallationsZone is the disagreement this seam exists
// to end.
//
// A close date is a calendar date a human picked, so whether it has passed is a
// question about the day it is where they work. Asked in UTC by a caller east
// of it, a date that is yesterday locally is still today — and the same record
// then reads overdue on the deal and on time in whatever re-derived it, for the
// whole working morning, every day.
func TestOverdueIsAskedInTheInstallationsZone(t *testing.T) {
	saigon, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("loading the zone: %v", err)
	}
	losAngeles, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("loading the zone: %v", err)
	}
	// 22:00 on 23 August UTC is 05:00 on the 24th in Saigon and 15:00 on the
	// 23rd in Los Angeles: one instant, three different days.
	now := time.Date(2026, 8, 23, 22, 0, 0, 0, time.UTC)
	expected := time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)

	if !CloseIsOverdue(expected, now, saigon) {
		t.Error("a deal expected on the 23rd is not overdue at 05:00 on the 24th in Saigon — " +
			"the working morning is exactly when a rep asks")
	}
	if CloseIsOverdue(expected, now, losAngeles) {
		t.Error("a deal expected on the 23rd is overdue at 15:00 on the 23rd in Los Angeles")
	}
	// UTC is the accidental zone, and it is what a caller gets for asking with
	// no installation zone at all.
	if CloseIsOverdue(expected, now, nil) {
		t.Error("a deal expected on the 23rd is overdue at 22:00 on the 23rd UTC")
	}
	// Strict: a deal closing today is not late anywhere.
	tomorrow := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	if CloseIsOverdue(tomorrow, now, saigon) {
		t.Error("a deal expected on the 24th is overdue on the 24th in Saigon")
	}
}
