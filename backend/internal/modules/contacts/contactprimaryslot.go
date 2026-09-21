// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The per-type primary slot: at most one of a contact's contact rows of a given
// type may be primary. uq_contact_email_primary and uq_contact_phone_primary hold
// that in the database; the check here holds the intent behind them, refusing a
// contradictory set before any write so the refusal can name the type instead of
// the bare conflict the index answers with.

import "github.com/margince/margince/backend/internal/shared/apperrors"

// PrimaryConflictError is a contact set that marks more than one row of a type
// primary — a contradiction the caller can fix. Kind is the noun for the
// message: "address" or "phone number".
type PrimaryConflictError struct {
	Kind string
	Type string
}

func (e *PrimaryConflictError) Error() string {
	return "only one " + e.Type + " " + e.Kind + " can be primary"
}

// Is reports this as the conflict sentinel, so every transport that maps
// ErrConflict to 409 keeps doing so without knowing this type exists.
func (e *PrimaryConflictError) Is(target error) bool { return target == apperrors.ErrConflict }

// ensureOnePrimaryPerType refuses rows that mark more than one member of a type
// primary. kind names the noun for the message; typeAndPrimary reads the two
// fields that differ between an address row and a phone row.
func ensureOnePrimaryPerType[T any](kind string, rows []T, typeAndPrimary func(T) (string, bool)) error {
	seen := map[string]bool{}
	for _, r := range rows {
		rowType, primary := typeAndPrimary(r)
		if !primary {
			continue
		}
		if seen[rowType] {
			return &PrimaryConflictError{Kind: kind, Type: rowType}
		}
		seen[rowType] = true
	}
	return nil
}
