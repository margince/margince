// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const employmentStatusField = "employment_status"

func validEmploymentAssertion(kind string, status, startedPrecision, endedPrecision *string) error {
	if status != nil {
		if kind != employmentKind || (*status != employmentCurrent && *status != employmentFormer && *status != employmentUnknown) {
			return &EmploymentImportError{Field: employmentStatusField, Message: "Choose current, former or unknown for an employment relationship."}
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

// refuseFormerEndingAhead refuses a former job whose end day is still ahead in
// the installation's day, the one IsCurrentSQL reads. A current job may end
// ahead: that is a notice period.
func refuseFormerEndingAhead(ctx context.Context, tx pgx.Tx, status *string, endedAt *time.Time) error {
	if status == nil || *status != employmentFormer || endedAt == nil {
		return nil
	}
	var ahead bool
	if err := tx.QueryRow(ctx, `SELECT $1::date > `+occurredday.TodaySQL("now()"), *endedAt).Scan(&ahead); err != nil {
		return fmt.Errorf("contacts: reading whether a former job ends ahead: %w", err)
	}
	if ahead {
		return &values.ParseError{
			Field: wireEndedAt, Code: "future",
			Message: "a former job ends on or before today; choose an earlier end date or mark the job current",
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
