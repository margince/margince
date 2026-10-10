// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "errors"

// requireReportingIDs names the id a read_reporting mode needs and the call did
// not send. The mode and the page size are held by the schema's `enum`,
// `minimum` and `maximum`.
func requireReportingIDs(in ReportingRead) error {
	switch in.Mode {
	case "report", "editions", "edition":
		if in.ID.IsZero() {
			return missingReportingID("id", in.Mode)
		}
	case "compare":
		if in.ID.IsZero() {
			return missingReportingID("id", in.Mode)
		}
		if in.RightID.IsZero() {
			return missingReportingID("right_id", in.Mode)
		}
	}
	return nil
}

func missingReportingID(field, mode string) error {
	return &BadArgsError{
		Cause: errors.New("`" + field + "` is required in mode " + mode),
		Field: field,
	}
}
