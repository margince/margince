// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func validEmploymentAssertion(kind string, status, startedPrecision, endedPrecision *string) error {
	if status != nil {
		if kind != employmentKind || (*status != employmentCurrent && *status != employmentFormer && *status != employmentUnknown) {
			return &EmploymentImportError{Field: "employment_status", Message: "Choose current, former or unknown for an employment relationship."}
		}
	}
	for _, field := range []struct {
		name  string
		value *string
	}{{"started_precision", startedPrecision}, {"ended_precision", endedPrecision}} {
		if field.value != nil && (kind != employmentKind || (*field.value != "day" && *field.value != "month")) {
			return &EmploymentImportError{Field: field.name, Message: "Employment date precision must be day or month."}
		}
	}
	return nil
}

func (e *employmentEvidence) correct(req crmcontracts.EmploymentImportRequest) error {
	if req.Started != nil {
		e.Started = strings.TrimSpace(*req.Started)
	}
	if req.Ended != nil {
		e.Ended = strings.TrimSpace(*req.Ended)
	}
	start, _ := preciseEmploymentDate(e.Started)
	end, _ := preciseEmploymentDate(e.Ended)
	e.invalidDates = (e.Started != "" && start == "") || (e.Ended != "" && end == "")
	if e.invalidDates || !e.validDateRange() {
		return &EmploymentImportError{Field: "ended", Message: "Use valid YYYY-MM or YYYY-MM-DD dates, with the end no earlier than the start."}
	}
	e.Started, e.Ended = start, end
	if req.EmploymentStatus != nil {
		e.Status = string(*req.EmploymentStatus)
	}
	return nil
}
