// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// DuplicateColumnError refuses a plan whose result would carry one column name
// twice. A row is keyed by column name, so the second value would replace the
// first and the row's derivation link would then carry the wrong one.
//
// MessageFault for the reason EmptyReportPlanError records: the contract
// declares one 422 code for this response.
type DuplicateColumnError struct{ Name string }

func (e *DuplicateColumnError) Error() string {
	return fmt.Sprintf("report: two result columns would both be called %s", httperr.QuoteCaller(e.Name))
}

// MessageFault names the fix: rename the aggregate.
func (e *DuplicateColumnError) MessageFault() (code, message string) {
	return reportFieldNotAllowedCode, e.Error() + " — name each `group_by` dimension once, and give each aggregate an `as` that no dimension or other aggregate uses"
}

// firstDuplicate is the first name that appears twice in columns, in plan order.
func firstDuplicate(columns []string) (string, bool) {
	seen := make(map[string]bool, len(columns))
	for _, name := range columns {
		if seen[name] {
			return name, true
		}
		seen[name] = true
	}
	return "", false
}

// The spellings a period bucket renders (periodBucketExpr and the fiscal
// labels beside it). A calendar and a fiscal spelling both exist because the
// installation's fiscal start decides which one a bucket takes.
var periodValueShapes = map[string]*regexp.Regexp{
	fieldPeriodYear:    regexp.MustCompile(`^(\d{4}|FY\d{4}/\d{2})$`),
	fieldPeriodQuarter: regexp.MustCompile(`^(\d{4}|FY\d{4}/\d{2})-Q[1-4]$`),
	fieldPeriodMonth:   regexp.MustCompile(`^\d{4}-(0[1-9]|1[0-2])$`),
}

// fiscalSpan reads the two years of a fiscal label, "FY2025/26".
var fiscalSpan = regexp.MustCompile(`^FY(\d{4})/(\d{2})`)

// PeriodValueError refuses a period filter no bucket can ever equal. Answered
// as a filter that matches nothing it reads as "no deals closed", which is a
// claim about the business made by a typo.
type PeriodValueError struct{ Filter, Value string }

func (e *PeriodValueError) Error() string {
	return fmt.Sprintf("report: %s is not a %s value", httperr.QuoteCaller(e.Value), e.Filter)
}

// MessageFault shows the spellings a bucket takes.
func (e *PeriodValueError) MessageFault() (code, message string) {
	return reportFieldNotAllowedCode, e.Error() + " — spell it like the buckets the report returns, such as 2026, 2026-Q1 or 2026-03"
}

// checkPeriodValue refuses a malformed value for a period filter; every other
// filter is left to its own check.
func checkPeriodValue(filter, value string) error {
	if shape, ok := periodValueShapes[filter]; !ok || (shape.MatchString(value) && fiscalSpanIsConsecutive(value)) {
		return nil
	}
	return &PeriodValueError{Filter: filter, Value: value}
}

// fiscalSpanIsConsecutive is true for a value with no fiscal span, and for a
// span ("FY2025/26") whose second year follows its first, the only span a
// bucket renders.
func fiscalSpanIsConsecutive(value string) bool {
	match := fiscalSpan.FindStringSubmatch(value)
	if match == nil {
		return true
	}
	start, errStart := strconv.Atoi(match[1])
	end, errEnd := strconv.Atoi(match[2])
	return errStart == nil && errEnd == nil && (start+1)%100 == end
}
