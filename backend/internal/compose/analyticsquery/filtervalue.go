// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

// What a filter value may be, and how a refusal names one that cannot bind.
//
// The driver encodes a Go string into a column of ANY type — text, numeric,
// boolean alike — and a JSON number or a true/false into no TEXT one. So every
// value has a text spelling that binds, and a value that is not text is
// admitted only where the field declares a column that can hold it.
//
// It is here rather than beside either door because both doors onto this
// engine ask the same question of the same argument, and a second phrasing of
// "quote it" would have the two surfaces advising a caller differently about
// one mistake.

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// QuoteAsTextAdvice is the fix, in the caller's own terms: the spelling that
// would have worked, not a taxonomy of accepted shapes to map a value onto.
const QuoteAsTextAdvice = `quote the value as text (a period like "2026" or "2026-Q1")`

// CannotCompare is the refusal's first half, shared by both doors: what
// arrived, and which filter would not take it.
func CannotCompare(shape, field string) string {
	return fmt.Sprintf("cannot compare %s against %s", shape, httperr.QuoteCaller(field))
}

// JSONShapeOf names an arrived value in the caller's own vocabulary.
//
// Deliberately NOT %T: the caller wrote JSON and has never heard of float64 or
// map[string]interface {}. Naming a Go type in a refusal both fails to locate
// their mistake and leaks how this server is built.
//
//craft:ignore naked-any it names the shape of a decoded JSON value, which is any by construction
func JSONShapeOf(value any) string {
	switch value.(type) {
	case float64, int, int64, json.Number:
		return "a number"
	case bool:
		return "a yes-or-no"
	case map[string]any:
		return "an object"
	case []any:
		return "a list"
	default:
		return "that value"
	}
}

// filterValueBinds answers whether this field can be compared against this
// value at all.
//
// Nothing is coerced. Rendering a number as its decimal string would make
// `2026` work against a period today and would silently compare "50" against
// the first numeric column a caller filters, which is a wrong answer rather
// than a refusal.
//
//craft:ignore naked-any a filter value arrives from caller JSON — judging what it may be is this function's whole job
func filterValueBinds(field Field, value any) bool {
	if _, text := value.(string); text {
		return true
	}
	switch field.Shape {
	case ShapeNumber:
		switch value.(type) {
		case float64, int, int64, json.Number:
			return true
		}
		return false
	case ShapeBoolean:
		_, yesOrNo := value.(bool)
		return yesOrNo
	default:
		return false
	}
}

// valueAdvice is the spelling that WOULD have bound, which is the half of a
// refusal a caller acts on. A boolean column is the case that makes this a
// function rather than one sentence: "quote it as text" is true of a number
// and false of a yes-or-no, and advice that does not work is a dead end
// dressed as help.
func valueAdvice(shape ColumnShape) string {
	switch shape {
	case ShapeNumber:
		return "send the number unquoted, or quote it as text"
	case ShapeBoolean:
		return "send true or false"
	default:
		return QuoteAsTextAdvice
	}
}
