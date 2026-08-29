// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The settings surface a rep sets their own autonomy from, and the loop it
// closes: what the read offers, whose rows it reads, and that a kind switched on
// here is the kind the sweep then applies without asking.
//
// SQL, every claim of it. Whether the read answers for kinds no row exists for,
// and whether one rep's switch is invisible to another, are both facts about
// what the query selects — a unit test with hand-built rows would agree with
// whatever the assembler believed.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// closeDateRow finds the close-date kind in a settings answer.
//
// That kind rather than any kind, because it is the one every test below acts
// on: it is the kind the auto-applier already ships an effect for, so a claim
// made about it is a claim about a path that runs.
func closeDateRow(t *testing.T, settings []approvals.KindAutonomy) approvals.KindAutonomy {
	t.Helper()
	for _, row := range settings {
		if row.Kind == deals.CloseDateCorrectionKind {
			return row
		}
	}
	t.Fatalf("the settings answer names no %q: %+v", deals.CloseDateCorrectionKind, settings)
	return approvals.KindAutonomy{}
}

// A rep who has never touched the feature is still offered every choice it has.
//
// The read returns the eligible SET rather than the rows the table holds, and
// this is the case that separates the two: with no policy row anywhere, a query
// that answered from the table would answer with nothing and the settings screen
// would offer a rep who has never decided anything no way to start.
func TestEveryAutomatableKindIsOfferedBeforeAnyoneHasDecidedOne(t *testing.T) {
	e := Setup(t)
	svc := approvals.NewService(e.DB())
	repCtx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	settings, err := svc.AutoApplySettings(repCtx)
	if err != nil {
		t.Fatalf("reading the autonomy settings: %v", err)
	}

	if len(settings) != len(approvals.AutoApplyKinds) {
		t.Fatalf("the read offers %d kinds, the product automates %d: %+v",
			len(settings), len(approvals.AutoApplyKinds), settings)
	}
	for _, row := range settings {
		if !approvals.AutoApplyKinds[row.Kind] {
			t.Errorf("offered %q, which is not a kind that can apply automatically", row.Kind)
		}
		if row.Mode != approvals.ModeManual {
			t.Errorf("%q reads %q for a rep who has decided nothing; a kind nobody has "+
				"chosen must read manual, which is what will happen to it",
				row.Kind, row.Mode)
		}
	}
}

// The setting a rep makes is the setting the read gives back, and the counters
// beside it are their own.
func TestSwitchingAKindOnIsWhatTheSettingsThenReport(t *testing.T) {
	e := Setup(t)
	svc := approvals.NewService(e.DB())
	repCtx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	if err := svc.SetAutoApply(repCtx, deals.CloseDateCorrectionKind, true); err != nil {
		t.Fatalf("switching the kind on: %v", err)
	}

	settings, err := svc.AutoApplySettings(repCtx)
	if err != nil {
		t.Fatalf("reading the autonomy settings: %v", err)
	}
	if got := closeDateRow(t, settings).Mode; got != approvals.ModeAuto {
		t.Fatalf("the kind the rep switched on reads %q", got)
	}
	// Everything else stays where it was: one switch is one kind.
	for _, row := range settings {
		if row.Kind == deals.CloseDateCorrectionKind {
			continue
		}
		if row.Mode != approvals.ModeManual {
			t.Errorf("%q moved to %q on a write that named a different kind",
				row.Kind, row.Mode)
		}
	}

	// And back off, because a setting a rep cannot reverse is not a setting.
	if err := svc.SetAutoApply(repCtx, deals.CloseDateCorrectionKind, false); err != nil {
		t.Fatalf("switching the kind back off: %v", err)
	}
	settings, err = svc.AutoApplySettings(repCtx)
	if err != nil {
		t.Fatalf("re-reading the autonomy settings: %v", err)
	}
	if got := closeDateRow(t, settings).Mode; got != approvals.ModeManual {
		t.Fatalf("after switching off, the kind reads %q", got)
	}
}

// One rep's answer is not another's.
//
// The read takes no user id, so the only thing standing between two reps is that
// the query keys on the principal. If it ever stopped doing so this test is what
// says, rather than a colleague silently inheriting an automatic apply they
// never agreed to.
func TestOneRepsAutonomyIsInvisibleToAnother(t *testing.T) {
	e := Setup(t)
	svc := approvals.NewService(e.DB())
	first := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	second := e.As(e.Rep2, []ids.UUID{e.Team1}, RepPerms)

	if err := svc.SetAutoApply(first, deals.CloseDateCorrectionKind, true); err != nil {
		t.Fatalf("switching the first rep's kind on: %v", err)
	}

	settings, err := svc.AutoApplySettings(second)
	if err != nil {
		t.Fatalf("reading the second rep's settings: %v", err)
	}
	if got := closeDateRow(t, settings).Mode; got != approvals.ModeManual {
		t.Fatalf("the second rep inherited %q from a colleague's choice", got)
	}
}

// The record a rep sees under the switch is the record of what they decided.
//
// Counted by the real decision path rather than an UPDATE, because the counters
// are written inside the decision transaction: a fixture that set them directly
// would prove the read works and say nothing about whether anything fills them.
func TestTheTrackRecordUnderASwitchCountsTheRepsOwnDecisions(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	svc := approvals.NewService(e.DB())
	deal := e.SeedDeal(t, "Fleet retrofit", pipeline, open, &e.Rep1)
	grantDealRepRole(t, e, e.Rep1)
	repCtx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	approvalID := stageCloseDateCorrection(t, svc, e, deal)
	if _, err := svc.Decide(repCtx, approvalID, false, nil); err != nil {
		t.Fatalf("rejecting the proposal: %v", err)
	}

	settings, err := svc.AutoApplySettings(repCtx)
	if err != nil {
		t.Fatalf("reading the autonomy settings: %v", err)
	}
	row := closeDateRow(t, settings)
	if row.Rejected != 1 {
		t.Fatalf("one rejection counted as %d rejections", row.Rejected)
	}
	if row.ApprovedClean != 0 || row.ApprovedEdited != 0 {
		t.Fatalf("a rejection counted as an approval: clean=%d edited=%d",
			row.ApprovedClean, row.ApprovedEdited)
	}
	// A rejection is not a reason to stop asking.
	if row.Mode != approvals.ModeManual {
		t.Fatalf("the kind reads %q after a rejection", row.Mode)
	}
}

// The whole point, end to end: the switch is what makes the sweep apply.
//
// This is the claim the feature exists for, and it is the one that was
// unprovable while nothing could write the mode — the engine read a setting no
// rep could reach, so the lane it fills stayed empty no matter how well the
// sweep worked. Setting it through the same method the endpoint calls is what
// makes this a test of the product rather than of a fixture.
func TestAKindSwitchedOnIsTheKindThatThenApplies(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	svc := approvals.NewService(e.DB())
	deal := e.SeedDeal(t, "Fleet retrofit", pipeline, open, &e.Rep1)
	grantDealRepRole(t, e, e.Rep1)
	approvalID := stageCloseDateCorrection(t, svc, e, deal)

	// Before the switch: the sweep leaves it alone.
	applied, err := compose.SweepAutoApply(sweepCtx(e), e.Pool)
	if err != nil {
		t.Fatalf("sweeping before the switch: %v", err)
	}
	if applied != 0 {
		t.Fatalf("the sweep applied %d proposals for a rep who has chosen nothing", applied)
	}
	if status, _ := statusOf(t, approvalID); status != "pending" {
		t.Fatalf("the proposal reads %q before anyone switched the kind on", status)
	}

	// The rep switches the kind on, through the method the endpoint calls.
	repCtx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	if err := svc.SetAutoApply(repCtx, deals.CloseDateCorrectionKind, true); err != nil {
		t.Fatalf("switching the kind on: %v", err)
	}

	applied, err = compose.SweepAutoApply(sweepCtx(e), e.Pool)
	if err != nil {
		t.Fatalf("sweeping after the switch: %v", err)
	}
	if applied != 1 {
		t.Fatalf("the sweep applied %d proposals after the rep switched the kind on", applied)
	}
	status, bySystem := statusOf(t, approvalID)
	if status != "approved" {
		t.Fatalf("the proposal reads %q after an automatic apply", status)
	}
	// The receipt has to say the machine did it, or the day's lane puts the
	// rep's name on a click they never made.
	if !bySystem {
		t.Fatal("an automatic apply is recorded as a person's decision")
	}
}
