// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The four dispositions never exceed the rows read.
//
// A finished run's stored report carries the dry run's skips as well as the
// commit's, because the engine folds attempts by object class and concatenates
// their skipped lists. Usually the same rows appear in both and dedup collapses
// them — but a row the preview skipped for a collision that then vanished
// commits as a CREATE, and counting both reports two outcomes for one row.
//
// The contract says the four sum to rows_read, and `unchanged` is derived by
// subtracting the other three: an overcount does not merely look wrong, it makes
// `unchanged` negative and the derivation clamps it to zero, so the report
// silently loses the surplus instead of showing it.

import (
	"testing"

	"github.com/gradionhq/margince/backend/internal/modules/migration"
)

func TestAFinishedDispositionNeverExceedsTheRowsRead(t *testing.T) {
	// One row: the preview skipped it for a collision, the commit created it
	// because the incumbent was gone by then. Both entries survive in the stored
	// report.
	run := migration.Run{
		Status: migration.StatusComplete,
		Report: &migration.Report{Objects: []migration.ObjectReport{{
			Object:      migration.ObjectOrganization,
			MirrorCount: 1,
			WillCreate:  0,
			Created:     1,
			Skipped: []migration.SkippedRow{{
				ExternalID: "Kestrel Data",
				Reason:     "a company of this name is already in the CRM",
			}},
		}}},
	}

	report := toContractImportReport(run)
	got := report.Disposition.Created + report.Disposition.Updated +
		report.Disposition.Unchanged + report.Disposition.Skipped
	if got != report.RowsRead {
		t.Errorf("the disposition sums to %d for %d rows read (created %d, updated %d, unchanged %d, "+
			"skipped %d) — the commit created this row and a stale preview skip was counted beside it",
			got, report.RowsRead, report.Disposition.Created, report.Disposition.Updated,
			report.Disposition.Unchanged, report.Disposition.Skipped)
	}
	if report.Disposition.Created != 1 {
		t.Errorf("created = %d, want 1 — the commit's own outcome is the one that happened",
			report.Disposition.Created)
	}
	// The issue survives even though the count does not: a person is still owed
	// the reason the preview gave.
	if len(report.Issues) != 1 {
		t.Errorf("%d issue(s), want 1 — dropping the entry hides why the preview said what it said",
			len(report.Issues))
	}
}
