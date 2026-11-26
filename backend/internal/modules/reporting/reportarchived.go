// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// codeReportArchived names the refusal of new work for an archived report.
const codeReportArchived = "report_archived"

// errReportArchived refuses a snapshot or a schedule for an archived report.
// The report and its saved snapshots stay readable, so it is a conflict with
// the report's state and never a missing report.
var errReportArchived error = reportArchivedError{}

type reportArchivedError struct{}

func (reportArchivedError) Error() string {
	return "this report is archived, so it takes no new snapshots or schedules; its saved snapshots stay readable"
}

func (reportArchivedError) Unwrap() error { return apperrors.ErrConflict }

func (e reportArchivedError) MessageFault() (code, message string) {
	return codeReportArchived, e.Error()
}

// refuseArchived answers errReportArchived for an archived report, nil otherwise.
func refuseArchived(report crmcontracts.ReportingReport) error {
	if report.ArchivedAt != nil {
		return errReportArchived
	}
	return nil
}
