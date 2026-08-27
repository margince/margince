// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// Setting a nullable field back to NOTHING, for every store that can.
//
// A JSON null cannot say "clear this" on its own: every field on every update
// request is an optional pointer, so a null decodes to nil and reads as "the
// caller did not supply this". Cleared fields therefore travel beside the patch
// as named fields, and each store declares which of its columns it will honour.
//
// This lives in storekit because Patch does: applying a clear is one assignment
// against the patch, and the refusal is one wire code. A module never imports a
// sibling, so a helper three stores need belongs in the layer all three already
// depend on rather than in three copies of itself.

// Clearable is one column a caller may set to NULL, and what the row holds there
// now. The current value is carried so the audit image says what the field was
// cleared FROM — after the write, the image is the only record of it.
//
//craft:ignore naked-any the value is whichever type the column holds; the patch seam takes it as the audit image does
type Clearable struct {
	Column  string
	Current any
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

// ApplyClears sets each named field to NULL, and refuses a name this store
// cannot clear. A field the map does not hold is either not nullable or not
// clearable through this path, and either way the honest answer is to say so
// rather than accept the instruction and drop it.
//
// It refuses BEFORE writing anything it has not already written, so a request
// naming one impossible field does not half-apply: the caller reads one error
// rather than a failure over a row that moved.
func ApplyClears(p *Patch, fields []string, columns map[string]Clearable) error {
	targets := make([]Clearable, 0, len(fields))
	for _, field := range fields {
		target, clearableHere := columns[field]
		if !clearableHere {
			return &NotClearableError{Field: field}
		}
		targets = append(targets, target)
	}
	for _, target := range targets {
		p.Set(target.Column, target.Current, nil)
	}
	return nil
}
