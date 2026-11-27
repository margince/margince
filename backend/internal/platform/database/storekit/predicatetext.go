// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "fmt"

// The text rule: a free-text condition ignores capitals, the spaces around the
// value and accents, as list search does. So "cto" finds "CTO" and "Muller"
// finds "Müller" whichever operator asks. A picklist holds option keys, compared
// as stored.

// foldsText reports whether a field compares under the text rule.
func foldsText(field Field) bool { return field.Type == FieldText }

// textFoldSQL is a text value as the rule compares it. The trigram indexes on
// names and addresses hold this expression, so a condition on one reads them.
func textFoldSQL(expr string) string { return "f_fold_apostrophes(lower(" + expr + "))" }

// textOperandSQL binds a text operand and folds it in SQL, where f_unaccent
// lives. A fold written again in Go would be a second answer to one question.
func textOperandSQL(value string, arg func(any) int) string {
	return textFoldSQL(fmt.Sprintf("$%d::text", arg(value)))
}

// comparedOperandSQL binds one operand on the side comparedExpr puts the
// column on, folded under the text rule for a text field.
//
//craft:ignore naked-any value is a checked filter operand, which spans the SQL scalar types
func comparedOperandSQL(field Field, value any, arg func(any) int) string {
	if text, ok := value.(string); ok && foldsText(field) {
		return textOperandSQL(text, arg)
	}
	return operandSQL(value, arg)
}

// membershipSQL binds the list of an `in`, each text operand folded as one is.
//
//craft:ignore naked-any values is the bind slice inOperand checked, typed per field
func membershipSQL(field Field, values any, arg func(any) int) string {
	if texts, ok := values.([]string); ok && foldsText(field) {
		return fmt.Sprintf("ANY(ARRAY(SELECT %s FROM unnest($%d::text[]) AS v))", textFoldSQL("v"), arg(texts))
	}
	return fmt.Sprintf("ANY($%d)", arg(values))
}
