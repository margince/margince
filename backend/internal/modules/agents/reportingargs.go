// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// reportingModes is the closed set of read_reporting modes. The `enum` tag on
// ReportingRead.Mode advertises the same words; a test holds the two equal.
var reportingModes = []string{"catalog", "evaluate", "reports", "report", "editions", "edition", "evidence", "compare"}

// Reporting list sizes, as the engine reads them. A page outside this range is
// the caller's number to change.
const (
	reportingMinLimit = 1
	reportingMaxLimit = 100
)

// requireReportingArgs names the argument a read_reporting call got wrong. The
// engine's bare "invalid argument" names no mode and no field.
func requireReportingArgs(in ReportingRead) error {
	if !slices.Contains(reportingModes, in.Mode) {
		return &BadArgsError{
			Cause:    fmt.Errorf("mode %q is not a reporting mode", in.Mode),
			Field:    "mode",
			Guidance: "the modes are: " + strings.Join(reportingModes, ", "),
		}
	}
	if in.Limit != 0 && (in.Limit < reportingMinLimit || in.Limit > reportingMaxLimit) {
		return &BadArgsError{
			Cause:    fmt.Errorf("limit is %d, outside %d to %d", in.Limit, reportingMinLimit, reportingMaxLimit),
			Field:    "limit",
			Guidance: "send a whole number from 1 to 100, or omit it for the default",
		}
	}
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
