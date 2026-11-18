// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

// A filter over a column of record ids takes only a canonical UUID. Bound as
// it is, any other text makes Postgres refuse the statement as a database
// fault, which the operator's log reads as a server error.

import (
	"fmt"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// IDValueAdvice is the spelling a filter over a column of record ids takes.
const IDValueAdvice = "send the record's id in the canonical 8-4-4-4-12 hex form"

// NotAnID is the first half of every door's refusal of a value that is no id.
func NotAnID(field, value string) string {
	return fmt.Sprintf("%s takes a record id, and %s is not one", httperr.QuoteCaller(field), httperr.QuoteCaller(value))
}

// idValueRefusal is the typed question's refusal of text that is no id.
//
//craft:ignore naked-any a filter value arrives from caller JSON
func idValueRefusal(field string, shape ColumnShape, value any) error {
	text, ok := value.(string)
	if shape != ShapeID || !ok {
		return nil
	}
	if _, err := ids.Parse(text); err == nil {
		return nil
	}
	return &RefusalError{Kind: RefusalInvalid, Message: "filters " + NotAnID(field, text), Suggest: IDValueAdvice}
}

// IDValueError is a report door's refusal of a value that is no id. Code is
// the 422 code that door's contract declares.
type IDValueError struct{ Filter, Value, Code string }

func (e *IDValueError) Error() string { return "report: filters " + NotAnID(e.Filter, e.Value) }

// MessageFault answers with the door's code and the spelling that binds.
func (e *IDValueError) MessageFault() (code, message string) {
	return e.Code, e.Error() + " — " + IDValueAdvice
}

// RequireIDValues refuses each filter value that the id column shapeOf names
// cannot hold, before any statement binds it. A nil value is a null check.
//
//craft:ignore naked-any filter values are the decoded JSON plan, schemaless by design
func RequireIDValues(filters map[string]any, code string, shapeOf func(name string) ColumnShape) error {
	for _, name := range slices.Sorted(maps.Keys(filters)) {
		value := filters[name]
		if value == nil || shapeOf(name) != ShapeID {
			continue
		}
		text, ok := value.(string)
		if _, err := ids.Parse(text); !ok || err != nil {
			return &IDValueError{Filter: name, Value: fmt.Sprint(value), Code: code}
		}
	}
	return nil
}
