// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The primary slot: at most one of a contact's email addresses, and at most one
// of its phone numbers, may be primary, whatever their type.
// uq_contact_email_primary and uq_contact_phone_primary hold that in the
// database; the check here holds the intent behind them, refusing a
// contradictory set before any write so the refusal can name the noun instead
// of the bare conflict the index answers with.

import "github.com/margince/margince/backend/internal/shared/apperrors"

// PrimaryConflictError is a contact set that marks more than one row primary,
// a contradiction the caller can fix. Kind is the noun for the message:
// "address" or "phone number".
type PrimaryConflictError struct {
	Kind string
}

func (e *PrimaryConflictError) Error() string {
	return "only one " + e.Kind + " can be primary"
}

// Is reports this as the conflict sentinel, so every transport that maps
// ErrConflict to 409 keeps doing so without knowing this type exists.
func (e *PrimaryConflictError) Is(target error) bool { return target == apperrors.ErrConflict }

// ensureOnePrimary refuses rows that mark more than one member primary. kind
// names the noun for the message.
func ensureOnePrimary[T any](kind string, rows []T, isPrimary func(T) bool) error {
	seen := false
	for _, r := range rows {
		if !isPrimary(r) {
			continue
		}
		if seen {
			return &PrimaryConflictError{Kind: kind}
		}
		seen = true
	}
	return nil
}

// primaryFirstIfNone marks the lowest-position row primary when none is, so a
// contact that has addresses always has one the app writes to. It is for a new
// contact, which has no stored primary to keep.
func primaryFirstIfNone[T any](rows []T, isPrimary func(T) bool, position func(T) int, mark func(*T)) {
	if len(rows) == 0 {
		return
	}
	first := 0
	for i := range rows {
		if isPrimary(rows[i]) {
			return
		}
		if position(rows[i]) < position(rows[first]) {
			first = i
		}
	}
	mark(&rows[first])
}
