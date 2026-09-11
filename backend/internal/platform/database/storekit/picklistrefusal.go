// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"strings"
)

// RefuseUnknownPicklistValues refuses a picklist leaf whose operand is not in
// the field's own set — at the moment a filter is STORED, and nowhere else.
//
// A picklist leaf compares text, so a typo compiles fine and selects nothing.
// That is an honest answer for a value no row carries, and it is the wrong
// answer for the contact who just typed it: the equivalent list PARAMETER
// answers 422 for the same mistake, so a saved view and a URL disagreed about
// whether "Open" is a status.
//
// WHY THIS IS NOT A COMPILER CHANGE, which is the whole of the decision it
// implements. Evaluation stays permissive: a definition stored last quarter
// keeps comparing text, so removing a value from a custom field's option set
// cannot make an existing view start failing at read time. Validating on write
// catches the typo at the moment somebody makes it, in the surface that made
// it, and no stored filter's behaviour changes.
//
// A SEPARATE FUNCTION rather than a strict flag on CompilePredicate, and that
// is deliberate too. The compiler has four callers — this write validator, the
// members evaluation, the filtered export and the preview count — and a `strict
// bool` would put the three read paths one wrong argument away from refusing a
// stored filter at read time, which is exactly what the decision avoids. A
// separate call cannot be armed on a read by accident, and cannot be disarmed
// on the write without deleting a line that says what it does.
//
// It also leaves the operand guarantee intact: values still travel unaltered as
// bind parameters. This refuses BEFORE compiling and rewrites nothing.
func RefuseUnknownPicklistValues(pred Predicate, fields map[string]Field) error {
	for _, branch := range pred.And {
		if err := RefuseUnknownPicklistValues(branch, fields); err != nil {
			return err
		}
	}
	for _, branch := range pred.Or {
		if err := RefuseUnknownPicklistValues(branch, fields); err != nil {
			return err
		}
	}
	if pred.Field == "" {
		return nil
	}
	field, known := fields[pred.Field]
	// An unknown field, and an operator this type does not admit, are the
	// compiler's refusals and it makes them a moment later with its own
	// messages. Answering here first would give one mistake two spellings.
	if !known || field.Type != FieldPicklist || len(field.Options) == 0 {
		return nil
	}
	switch pred.Op {
	case OpEq, OpNeq:
		return refuseStranger(pred.Field, pred.Value, field.Options)
	case OpIn:
		members, ok := pred.Value.([]any)
		if !ok {
			// The shape is the compiler's business: `in` with a scalar is its
			// refusal to make, in its own words.
			return nil
		}
		for _, member := range members {
			if err := refuseStranger(pred.Field, member, field.Options); err != nil {
				return err
			}
		}
	}
	// Every other operator is unreachable on a picklist — operatorsByType
	// admits only these four plus `exists` — so there is no fallback branch to
	// write. `exists` is skipped because its operand is the QUESTION rather
	// than a value: "is this set at all" names no member of the set.
	return nil
}

// refuseStranger names the first value outside the set, and the set.
//
// The set is in the message because a picklist's values are the whole of what a
// caller needs to correct the mistake, and they are already published to the
// same client through the field vocabulary — so this discloses nothing a reader
// could not ask for, and saves them a second request to find out what they
// should have typed.
//
//craft:ignore naked-any the operand is Predicate.Value, schemaless at the wire — a JSON scalar or array, typed only by the field it names
func refuseStranger(field string, value any, options []string) error {
	text, ok := value.(string)
	if !ok {
		// A non-string operand on a picklist is the type check's refusal.
		return nil
	}
	for _, option := range options {
		if text == option {
			return nil
		}
	}
	return &PredicateError{
		Field:   field,
		Code:    "filter_value_invalid",
		Message: fmt.Sprintf("%q is not a value of %s (%s)", text, field, strings.Join(options, ", ")),
	}
}
