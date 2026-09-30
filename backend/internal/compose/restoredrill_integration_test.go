// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A drill that is not written down proves nothing afterwards, which is exactly
// when somebody asks.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/continuity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The whole point: a rehearsal leaves evidence, and both published numbers
// come out of it.
func TestADrillLeavesTheTwoNumbersItWasRunToProduce(t *testing.T) {
	e := integration.Setup(t)
	store := continuity.NewStore(InstallationDB(e.Pool))

	// A backup taken half an hour before the failure it stands in for.
	restoredTo := time.Now().Add(-30 * time.Minute)
	id, err := store.Begin(e.Admin(), restoredTo, "quarterly rehearsal")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := store.Finish(e.Admin(), id, continuity.OutcomePassed, "verified against the seeded workspace"); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	drill, ever, err := store.Latest(e.Admin())
	if err != nil || !ever {
		t.Fatalf("Latest: %v (ever=%v)", err, ever)
	}
	if drill.Outcome != continuity.OutcomePassed {
		t.Errorf("outcome = %q, want passed", drill.Outcome)
	}
	// The data-loss window is what the hourly claim is about.
	if loss := drill.DataLossWindow(); loss < 29*time.Minute || loss > 31*time.Minute {
		t.Errorf("data-loss window = %s, want about half an hour", loss)
	}
	// The recovery window is known because the drill finished.
	if _, known := drill.RecoveryWindow(); !known {
		t.Error("a finished drill reports no recovery window")
	}
	if drill.Operator == "" {
		t.Error("the drill records no operator")
	}
}

// An installation that has never rehearsed has to be able to say so. It is the
// case the published numbers are least true of, and a surface that could not
// express it would read as a drill that passed.
func TestAnInstallationThatHasNeverDrilledSaysSo(t *testing.T) {
	e := integration.Setup(t)

	_, ever, err := continuity.NewStore(InstallationDB(e.Pool)).Latest(e.Admin())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if ever {
		t.Fatal("an installation with no drills reported one")
	}
}

// A failed drill is evidence too. A ledger that only recorded its successes
// would be a record of nothing, and the failure is the reading somebody most
// needs before relying on the procedure.
func TestAFailedDrillIsRecorded(t *testing.T) {
	e := integration.Setup(t)
	store := continuity.NewStore(InstallationDB(e.Pool))

	id, err := store.Begin(e.Admin(), time.Now().Add(-time.Hour), "")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := store.Finish(e.Admin(), id, continuity.OutcomeFailed, "the object store was not reachable"); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	drill, _, err := store.Latest(e.Admin())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if drill.Outcome != continuity.OutcomeFailed {
		t.Fatalf("outcome = %q, want the failure kept", drill.Outcome)
	}
	if drill.Notes == "" {
		t.Error("the failure records no reason")
	}
}

// A drill still running has no recovery window — which is different from a
// window of zero, and different again from never having drilled.
func TestARunningDrillReportsNoRecoveryWindowYet(t *testing.T) {
	e := integration.Setup(t)
	store := continuity.NewStore(InstallationDB(e.Pool))

	if _, err := store.Begin(e.Admin(), time.Now().Add(-time.Minute), ""); err != nil {
		t.Fatalf("Begin: %v", err)
	}
	drill, ever, err := store.Latest(e.Admin())
	if err != nil || !ever {
		t.Fatalf("Latest: %v (ever=%v)", err, ever)
	}
	if _, known := drill.RecoveryWindow(); known {
		t.Fatal("a drill still running reported a recovery window")
	}
	if drill.Outcome != continuity.OutcomeRunning {
		t.Errorf("outcome = %q, want running", drill.Outcome)
	}
}

// The first answer is the one that happened. Letting a second close overwrite
// it would make the ledger whatever its last writer preferred.
func TestADrillCannotBeClosedTwice(t *testing.T) {
	e := integration.Setup(t)
	store := continuity.NewStore(InstallationDB(e.Pool))

	id, err := store.Begin(e.Admin(), time.Now().Add(-time.Minute), "")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := store.Finish(e.Admin(), id, continuity.OutcomeFailed, "ran out of time"); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := store.Finish(e.Admin(), id, continuity.OutcomePassed, "tried again"); err == nil {
		t.Fatal("a closed drill was re-closed as passed")
	}

	drill, _, err := store.Latest(e.Admin())
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if drill.Outcome != continuity.OutcomeFailed {
		t.Fatalf("outcome = %q, want the first answer kept", drill.Outcome)
	}
}

// Recording a rehearsal is an operator's act, not a reader's.
func TestRecordingADrillNeedsTheSettingsGrant(t *testing.T) {
	e := integration.Setup(t)
	store := continuity.NewStore(InstallationDB(e.Pool))

	_, err := store.Begin(e.As(e.Rep1, nil, integration.AccountRepPerms), time.Now(), "")
	if err == nil {
		t.Fatal("a seat without the installation-settings grant recorded a drill")
	}
	_ = apperrors.ErrPermissionDenied
}

// An outcome the ledger does not define is refused rather than stored: a
// vocabulary nobody agreed to is one nothing can read back.
func TestADrillRefusesAnOutcomeItDoesNotDefine(t *testing.T) {
	e := integration.Setup(t)
	store := continuity.NewStore(InstallationDB(e.Pool))

	id, err := store.Begin(e.Admin(), time.Now().Add(-time.Minute), "")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := store.Finish(e.Admin(), id, "mostly fine", ""); err == nil {
		t.Fatal("an undefined outcome was accepted")
	}
}
