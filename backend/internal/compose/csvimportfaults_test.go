// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
)

// A row with two cells at fault is two issues and one skipped row. The commit
// skips it again with the same two cells, and the report still names each once.
func TestARowWithTwoFaultsIsTwoIssuesAndOneSkip(t *testing.T) {
	amount := migration.SkippedRow{ExternalID: "D-1", Line: 2, Column: "amount", Kind: migration.ProblemInvalidValue, Value: "1.234,56"}
	date := migration.SkippedRow{ExternalID: "D-1", Line: 2, Column: "close_date", Kind: migration.ProblemInvalidValue, Value: "31.03.2027"}
	clean := migration.SkippedRow{ExternalID: "D-2", Line: 3, Column: "amount", Kind: migration.ProblemInvalidValue, Value: "x"}
	for status, skipped := range map[string][]migration.SkippedRow{
		migration.StatusAwaitingApproval: {amount, date, clean},
		migration.StatusComplete:         {amount, date, clean, amount, date, clean},
	} {
		run := migration.Run{Status: status, Report: &migration.Report{Objects: []migration.ObjectReport{{
			Object: migration.ObjectDeal, MirrorCount: 3, WillCreate: 1, Created: 1, Skipped: skipped,
		}}}}
		report := toContractImportReport(run)
		var columns []string
		for _, issue := range report.Issues {
			columns = append(columns, *issue.Column)
		}
		if strings.Join(columns, ",") != "amount,close_date,amount" || report.Disposition.Skipped != 2 {
			t.Errorf("%s: issues on %v, %d skipped; want three issues on two skipped rows", status, columns, report.Disposition.Skipped)
		}
	}
}

// A test run has written nothing, so its dropped link is still to come. A run
// that has written says it was dropped, in the sentence and the code.
func TestADroppedLinkIsReportedInTheTenseOfItsRun(t *testing.T) {
	why := migration.Why(migration.ReasonLinkNotFoundDropped, "ref", "CO-404")
	for status, want := range map[string]migration.ReasonCode{
		migration.StatusAwaitingApproval: migration.ReasonLinkNotFoundWillDrop,
		migration.StatusComplete:         migration.ReasonLinkNotFoundDropped,
		migration.StatusFailed:           migration.ReasonLinkNotFoundDropped,
	} {
		warning := migration.SkippedRow{ExternalID: "C-1", Line: 2, Column: "company", Kind: migration.ProblemDroppedLink}.Because(why)
		run := migration.Run{Status: status, Checkpoint: 1, Report: &migration.Report{Objects: []migration.ObjectReport{{
			Object: migration.ObjectContact, MirrorCount: 1, WillCreate: 1, Created: 1, Warnings: []migration.SkippedRow{warning},
		}}}}
		issues := toContractImportReport(run).Issues
		if len(issues) != 1 || issues[0].ReasonCode == nil || string(*issues[0].ReasonCode) != string(want) ||
			issues[0].Reason != migration.Why(want, "ref", "CO-404").English() {
			t.Errorf("%s: issues = %+v, want the link reported as %s", status, issues, want)
		}
	}
}
