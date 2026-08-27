// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a deal restore can set back to NOTHING, and the machinery for doing it.
// Its own file for the reason the people module keeps the same concept in one:
// "which fields this store can clear" is a question a reader asks whole, and
// answering it means reading one place rather than hunting through the writer.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// clearable is one column a caller may set to NULL, and what the row holds
// there now. The current value is carried so the audit image says what the
// field was cleared FROM.
//
//craft:ignore naked-any the value is whichever type the column holds; the patch seam takes it as the audit image does
type clearable struct {
	column  string
	current any
}

// NotClearableError refuses an explicit null on a field this record cannot set
// to nothing. It maps to 422 through the FieldFault seam.
//
// Refusing matters: the caller sent a null on a field the contract declares
// nullable, so ignoring it would answer 200 having changed nothing — a success
// they cannot trust.
type NotClearableError struct{ Field string }

func (e *NotClearableError) Error() string {
	return e.Field + " cannot be set to null on this record; omit the field to leave it unchanged"
}

// FieldFault names the field the caller tried to clear.
func (e *NotClearableError) FieldFault() (field, code, message string) {
	return e.Field, "field_not_clearable", e.Error()
}

// applyClears sets each named field to NULL, and refuses a name this store
// cannot clear. A field the map does not hold is either not nullable or not
// clearable through this path, and either way the honest answer is to say so
// rather than accept the instruction and drop it.
func applyClears(p *storekit.Patch, fields []string, columns map[string]clearable) error {
	for _, field := range fields {
		target, clearableHere := columns[field]
		if !clearableHere {
			return &NotClearableError{Field: field}
		}
		p.Set(target.column, target.current, nil)
	}
	return nil
}

// clearableDealColumns names the wire fields a deal restore may set to NULL,
// with literal column names. amount_minor and currency are absent: money is
// read as one field, and a half-cleared pair states an amount in no currency.
// status and the close-date flags belong to the advance path.
//
//nolint:goconst // wire field names against column names, each its own vocabulary — see clearablePersonColumns
func clearableDealColumns(current crmcontracts.Deal) map[string]clearable {
	return map[string]clearable{
		"expected_close_date": {"expected_close_date", current.ExpectedCloseDate},
		"forecast_category":   {"forecast_category", current.ForecastCategory},
		"wait_until":          {"wait_until", current.WaitUntil},
		"owner_id":            {"owner_id", current.OwnerId},
		"project_id":          {"project_id", current.ProjectId},
	}
}
