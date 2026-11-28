// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importwire

import (
	"fmt"

	"github.com/margince/margince/backend/internal/modules/migration"
)

// WithWarnings adds warnings that belong to one part rather than to a row.
func WithWarnings(report migration.Report, object string, warnings []migration.SkippedRow) migration.Report {
	for i := range report.Objects {
		if report.Objects[i].Object == object {
			report.Objects[i].Warnings = append(report.Objects[i].Warnings, warnings...)
		}
	}
	return report
}

// WithSkippedLines folds the source's own disclosures into the object's report.
// A row the source could not deliver never reached the writer, so nothing else
// in the report would mention it.
func WithSkippedLines(report migration.Report, object string, skipped []migration.SkippedLine) migration.Report {
	if len(skipped) == 0 {
		return report
	}
	for i := range report.Objects {
		if report.Objects[i].Object != object {
			continue
		}
		for _, s := range skipped {
			report.Objects[i].Skipped = append(report.Objects[i].Skipped, migration.SkippedRow{
				ExternalID: fmt.Sprintf("line %d", s.Line),
				Line:       s.Line,
				Reason:     s.Reason,
				Code:       s.Why.Code,
				Params:     s.Why.Params,
			})
		}
	}
	return report
}
