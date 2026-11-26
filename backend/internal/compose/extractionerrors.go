// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// UnsupportedEntityTypeError maps to 422 unsupported_entity_type:
// accepting extraction fields writes a deal, and an attachment scoped to
// any other parent has no deal to write.
type UnsupportedEntityTypeError struct{ EntityType string }

func (e *UnsupportedEntityTypeError) Error() string {
	return "extraction accept is only valid on a deal-scoped attachment, not " + e.EntityType
}

// MessageFault carries the 422 on the error, so the one taxonomy answers it
// wherever it travels rather than only where writeExtractionAcceptError runs.
//
// MessageFault, not FieldFault: what is wrong is the attachment's scope, and
// no argument of this request can change it. Pointing at a field would hand the
// caller an edit it cannot make.
func (e *UnsupportedEntityTypeError) MessageFault() (code, message string) {
	return "unsupported_entity_type", e.Error()
}

// ExtractionAcceptError is one refused accept input: the whole request
// refuses (no partial acceptance), naming the offending field and the
// machine code.
type ExtractionAcceptError struct{ Field, Code, Message string }

func (e *ExtractionAcceptError) Error() string { return e.Field + ": " + e.Message }

// FieldFault carries the verdict the type already holds. It names a real
// argument of the request, which is what separates it from
// UnsupportedEntityTypeError next door.
func (e *ExtractionAcceptError) FieldFault() (field, code, message string) {
	return e.Field, e.Code, e.Message
}
