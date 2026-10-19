// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// EmptyEntryError maps to 422. A note or a call is what its subject or body
// says. An entry with neither puts a line on the timeline that tells no one
// what happened.
type EmptyEntryError struct{ Kind string }

func (e *EmptyEntryError) Error() string {
	return "a " + e.Kind + " needs a subject or a body; an entry with neither says nothing on the timeline"
}

// FieldFault names the subject, the field a caller fixes first.
func (e *EmptyEntryError) FieldFault() (field, code, message string) {
	return fieldSubject, "subject_or_body_required", e.Error()
}

// CheckEntryHasContent is the one rule for an entry that must say something.
// The store holds it on every create and on a patch of a stored entry. The
// create wire and an import ask it first so a row is refused for its cells
// before any write. Only a note and a call are held to it.
func CheckEntryHasContent(kind string, subject, body *string) error {
	if kind != string(crmcontracts.ActivityKindNote) && kind != string(crmcontracts.ActivityKindCall) {
		return nil
	}
	if isBlank(subject) && isBlank(body) {
		return &EmptyEntryError{Kind: kind}
	}
	return nil
}

func isBlank(text *string) bool {
	return text == nil || strings.TrimSpace(*text) == ""
}

// firstSet is what a coalescing patch leaves in a column: the patched value
// when the patch names one, the stored value otherwise.
func firstSet(patched, stored *string) *string {
	if patched != nil {
		return patched
	}
	return stored
}
