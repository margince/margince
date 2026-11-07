// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// The pass reads a year of mail, so it runs as seldom as readers allow. Two
// missed passes must still leave a value younger than the staleness limit. A
// pass twice as often as that is work nobody reads.
func TestTheRefreshPassRunsAsSeldomAsTheStalenessLimitAllows(t *testing.T) {
	spec, ok := jobs.SpecFor(WaitingHorizonRefreshArgs{}.Kind())
	if !ok {
		t.Fatal("waiting_horizon_refresh declares no job spec")
	}
	cadence := spec.Cadence.Fixed
	if cadence <= 0 {
		t.Fatalf("waiting_horizon_refresh has no fixed cadence: %+v", spec.Cadence)
	}
	if 3*cadence >= storekit.WaitingHorizonStaleAfter {
		t.Errorf("every %s: two missed passes leave a value as old as the %s limit", cadence, storekit.WaitingHorizonStaleAfter)
	}
	if 6*cadence < storekit.WaitingHorizonStaleAfter {
		t.Errorf("every %s: the pass runs more than twice as often as two misses need", cadence)
	}
}

// The measurement is a year of mail, so its budget must outlast the general
// ceiling and still end inside the job's own timeout.
func TestTheRefreshPassBudgetOutlastsTheStatementCeilingAndFitsTheJob(t *testing.T) {
	budget := waitingHorizonStatementBudget()
	spec, ok := jobs.SpecFor(WaitingHorizonRefreshArgs{}.Kind())
	if !ok {
		t.Fatal("waiting_horizon_refresh declares no job spec")
	}
	if budget <= database.StatementCeiling || budget >= spec.Timeout.Duration() {
		t.Fatalf("budget %s must sit between the %s ceiling and the job's %s timeout",
			budget, database.StatementCeiling, spec.Timeout.Duration())
	}
}

func TestAKindWithoutADeclaredSpecHasNoStatementBudget(t *testing.T) {
	if _, err := statementBudgetFor("no_such_kind"); err == nil {
		t.Fatal("an undeclared kind was given a statement budget")
	}
}

func TestAnUndeclaredKindPanicsRatherThanRunUnbounded(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("an undeclared kind was given a statement budget")
		}
	}()
	mustStatementBudgetFor("no_such_kind")
}
