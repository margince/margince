// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

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
