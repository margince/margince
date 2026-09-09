// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// One JSON operand becomes one bound parameter, under the FIELD's kind rather
// than the JSON's own shape.
//
// Split out of the statement assembly because it answers a different question:
// querysql.go decides what the statement says, and this decides what a value
// the caller wrote means when the column it is compared against says it is a
// date, a uuid or a number. A refusal here names the operand, not the clause.

import (
	"encoding/json"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bind turns one JSON operand into a bound parameter under the field's kind,
// with the cast the comparison needs.
//
// This is where a FORMAT is checked. The validator deliberately left it here
// ("their format is the executor's business"): it had a shape to compare
// against and no calendar, and refusing `"next tuesday"` at the moment it
// would become a parameter is what keeps a malformed date a refusal rather
// than a query that quietly matches nothing.
//
//craft:ignore naked-any a bound parameter is whatever Go type the column's kind encodes as (string, int64, float64, bool, time.Time, ids.UUID) — the kind switch IS the conversion contract, so there is no narrower signature
func (c *planCompiler) bind(at string, field Field, raw json.RawMessage) (any, string, *apperrors.FieldRefusal) {
	switch field.Kind {
	case KindNumber:
		return bindNumber(at, field, raw)
	case KindBoolean:
		var value bool
		return value, "", decodeOperand(at, field, raw, &value, "true or false")
	case KindID:
		return bindID(at, field, raw)
	case KindDate:
		return bindTemporal(at, field, raw, dateLayout, "::date", "a date, as YYYY-MM-DD")
	case KindTimestamp:
		return bindTemporal(at, field, raw, time.RFC3339, "", "an instant, as RFC 3339 (2026-08-08T09:00:00Z)")
	case KindText:
		var value string
		return value, "", decodeOperand(at, field, raw, &value, "text")
	case KindGeo:
		// A place is never compared: within_radius answers
		// distance_ranking_unavailable and the executor stops before here.
		return nil, "", operandFault(at, field, "a place, which this deployment cannot rank by")
	default:
		return nil, "", operandFault(at, field, "a value of a kind this workspace can compare")
	}
}

//craft:ignore naked-any a bound parameter is whatever Go type the column's kind encodes as (string, int64, float64, bool, time.Time, ids.UUID) — the kind switch IS the conversion contract, so there is no narrower signature
func bindNumber(at string, field Field, raw json.RawMessage) (any, string, *apperrors.FieldRefusal) {
	var value json.Number
	if refusal := decodeOperand(at, field, raw, &value, "a number"); refusal != nil {
		return nil, "", refusal
	}
	// A whole number binds as one, so a bigint column compares against a
	// bigint rather than against a float that rounded on the way in.
	if whole, err := value.Int64(); err == nil {
		return whole, "", nil
	}
	// A FRACTIONAL number binds as its own digits, cast to numeric. Through a
	// float64 it would not: 0.1 is not representable in binary, so a `numeric`
	// column holding exactly 0.1 would compare unequal to the 0.1 the caller
	// wrote — an exact predicate answering "no rows" for a value that is
	// there. The digits are the caller's own text and go through a bind
	// parameter, so nothing is interpolated.
	if _, err := value.Float64(); err != nil {
		return nil, "", operandFault(at, field, "a number")
	}
	return value.String(), "::numeric", nil
}

//craft:ignore naked-any a bound parameter is whatever Go type the column's kind encodes as (string, int64, float64, bool, time.Time, ids.UUID) — the kind switch IS the conversion contract, so there is no narrower signature
func bindID(at string, field Field, raw json.RawMessage) (any, string, *apperrors.FieldRefusal) {
	var text string
	if refusal := decodeOperand(at, field, raw, &text, "an identifier"); refusal != nil {
		return nil, "", refusal
	}
	id, err := ids.Parse(text)
	if err != nil {
		return nil, "", operandFault(at, field, "an identifier, as a UUID")
	}
	return id, "", nil
}

// bindTemporal parses a date or an instant in the contract's own encoding and
// binds it as TEXT with an explicit cast where one is needed. A date compared
// through a timestamp would be resolved at the session's time zone, which
// makes the same plan answer differently on two servers.
//
//craft:ignore naked-any a bound parameter is whatever Go type the column's kind encodes as (string, int64, float64, bool, time.Time, ids.UUID) — the kind switch IS the conversion contract, so there is no narrower signature
func bindTemporal(at string, field Field, raw json.RawMessage, layout, cast, shape string) (any, string, *apperrors.FieldRefusal) {
	var text string
	if refusal := decodeOperand(at, field, raw, &text, shape); refusal != nil {
		return nil, "", refusal
	}
	parsed, err := time.Parse(layout, text)
	if err != nil {
		return nil, "", operandFault(at, field, shape)
	}
	if cast == "" {
		return parsed, "", nil
	}
	return parsed.Format(layout), cast, nil
}

// decodeOperand decodes one operand into its Go type, which IS the check: a
// number offered where text belongs fails at the decode rather than after it.
//
//craft:ignore naked-any `into` is the caller's own destination for one operand; decoding INTO its Go type is the check, and a narrower signature would have to name every kind
func decodeOperand(at string, field Field, raw json.RawMessage, into any, shape string) *apperrors.FieldRefusal {
	if len(raw) == 0 || isJSONNull(raw) || json.Unmarshal(raw, into) != nil {
		return operandFault(at, field, shape)
	}
	return nil
}

func operandFault(at string, field Field, shape string) *apperrors.FieldRefusal {
	return &apperrors.FieldRefusal{
		Field: at, Code: CodeValueTypeMismatch,
		Message: quote(field.Name) + " is a " + string(field.Kind) + " field; its operand must be " + shape,
	}
}
