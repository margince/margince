// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import "unicode/utf8"

// SourceIDMax bounds source_system and source_id, in runes. It is the cap
// capture_trace.source_system carries and the length CHECK on every source_id
// column.
const SourceIDMax = 128

// SourceIDError refuses the outside-identity pair on a wire, naming the field
// it arrived on. It is caller-fixable, so every surface answers it as a 422.
type SourceIDError struct{ Field, Code, Message string }

func (e *SourceIDError) Error() string { return e.Message }

// FieldFault states the refusal for the HTTP mapper and the tool surface alike.
func (e *SourceIDError) FieldFault() (field, code, message string) {
	return e.Field, e.Code, e.Message
}

// CheckSourceValues refuses a value no record can hold: a source_system past the
// cap, or a source_id that is empty or past it. A create and a list both ask it,
// so they agree on what a value may be.
func CheckSourceValues(sourceSystem, sourceID *string) error {
	if sourceSystem != nil && utf8.RuneCountInString(*sourceSystem) > SourceIDMax {
		return &SourceIDError{
			Field: "source_system", Code: "source_system_invalid",
			Message: "source_system must be at most 128 characters",
		}
	}
	if sourceID != nil {
		if n := utf8.RuneCountInString(*sourceID); n == 0 || n > SourceIDMax {
			return &SourceIDError{
				Field: "source_id", Code: "source_id_invalid",
				Message: "source_id must be a non-empty id of at most 128 characters",
			}
		}
	}
	return nil
}

// AdmitSourceID guards the outside identity of a create wire. A source_id says
// which record this was in source_system, so it needs the system. Both are
// stored as sent, because a lookup matches them byte for byte.
func AdmitSourceID(sourceSystem, sourceID *string) error {
	if err := CheckSourceValues(sourceSystem, sourceID); err != nil {
		return err
	}
	if sourceID != nil && (sourceSystem == nil || *sourceSystem == "") {
		return &SourceIDError{
			Field: "source_id", Code: "source_id_needs_a_source",
			Message: "source_id needs source_system: an id only means something inside the system that issued it",
		}
	}
	return nil
}
