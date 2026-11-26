// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importwire

// The rows a run's report names. The dry run and the commit may both refuse a
// row, and the report names it once.

import (
	"regexp"
	"strconv"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// IssueKey is what makes two reported rows one. The object joins it because
// two files of a ZIP may each have a row `line 7`. The line joins the id
// because a file's key column may hold the text a source disclosure uses.
func IssueKey(o migration.ObjectReport, row migration.SkippedRow) string {
	return o.Object + "\x00" + row.ExternalID + "\x00" + strconv.Itoa(row.Line)
}

// SkipKey is IssueKey with the column, so each cell a skipped row has to have
// corrected is named. The dry run's skip and the commit's still fold to one.
func SkipKey(o migration.ObjectReport, row migration.SkippedRow) string {
	return IssueKey(o, row) + "\x00" + row.Column
}

// WarningKey is IssueKey with the kind and value. One row may carry several
// warnings, and a part's new tags belong to no row at all.
func WarningKey(o migration.ObjectReport, row migration.SkippedRow) string {
	return IssueKey(o, row) + "\x00" + row.Kind + "\x00" + row.Value
}

// AppendIssues adds one object's reported rows to issues, each key once.
func AppendIssues(
	issues *[]crmcontracts.ImportRowIssue, seen map[string]bool,
	keyOf func(migration.ObjectReport, migration.SkippedRow) string, o migration.ObjectReport, rows []migration.SkippedRow,
) {
	for _, row := range rows {
		key := keyOf(o, row)
		if seen[key] {
			continue
		}
		seen[key] = true
		*issues = append(*issues, crmcontracts.ImportRowIssue{
			Line:       lineOrAbsent(lineOf(row)),
			Reason:     row.Reason,
			ReasonCode: ReasonCodeOrAbsent(row.Code),
			Params:     ParamsOrAbsent(row.Params),
			File:       TextOrAbsent(o.File),
			Column:     TextOrAbsent(row.Column),
			Kind:       kindOrAbsent(row.Kind),
			Value:      TextOrAbsent(row.Value),
		})
	}
}

// DistinctIssues counts the rows AppendIssues names once each.
func DistinctIssues(o migration.ObjectReport, rows []migration.SkippedRow) int {
	seen := map[string]bool{}
	for _, row := range rows {
		seen[IssueKey(o, row)] = true
	}
	return len(seen)
}

// lineOf is the file line a skip names. A report stored without the line
// still names it in the id of a row the source disclosed.
func lineOf(skip migration.SkippedRow) int {
	if skip.Line != 0 {
		return skip.Line
	}
	match := disclosedLine.FindStringSubmatch(skip.ExternalID)
	if match == nil {
		return 0
	}
	line, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return line
}

// disclosedLine is the whole id the source writes for a row it could not
// identify. A mirror source's ids are the incumbent's, so "line 7 of the
// export" is no line.
var disclosedLine = regexp.MustCompile(`^line ([0-9]+)$`)

// lineOrAbsent leaves out the line of a problem that names no row, such as a
// tag the whole file refers to.
func lineOrAbsent(line int) *int {
	if line == 0 {
		return nil
	}
	return &line
}

// TextOrAbsent leaves an empty text off the wire.
func TextOrAbsent(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

func kindOrAbsent(kind string) *crmcontracts.ImportProblemKind {
	if kind == "" {
		return nil
	}
	k := crmcontracts.ImportProblemKind(kind)
	return &k
}

// ReasonCodeOrAbsent is a report row's code, or nil for a row stored without one.
func ReasonCodeOrAbsent(code migration.ReasonCode) *crmcontracts.ImportReasonCode {
	if code == "" {
		return nil
	}
	return new(crmcontracts.ImportReasonCode(code))
}

// ParamsOrAbsent leaves a reason with no values off the wire.
func ParamsOrAbsent(params migration.Params) *map[string]string {
	if len(params) == 0 {
		return nil
	}
	out := map[string]string(params)
	return &out
}
