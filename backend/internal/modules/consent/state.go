// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// State is the per-purpose consent vocabulary — the Go spelling of the
// person_consent state CHECK (0010), kept in sync by the enumsync
// fitness gate. Unknown and withdrawn both suppress (default-deny);
// only a proven granted authorizes an outbound action.
type ConsentState string

const (
	StateUnknown   ConsentState = "unknown"
	StateGranted   ConsentState = "granted"
	StateWithdrawn ConsentState = "withdrawn"
)

// ParseRecordableState guards the record seam: a client records a grant
// or a withdrawal; "unknown" is the absence of a decision, never an
// input (consent_event's own CHECK carries the same two-value rule).
func ParseRecordableState(raw string) (ConsentState, error) {
	switch s := ConsentState(raw); s {
	case StateGranted, StateWithdrawn:
		return s, nil
	}
	return "", &values.ParseError{Field: "state", Code: "invalid_consent_state",
		Message: "state is granted or withdrawn"}
}

// normalizedPurposeKey is the ONE spelling of how a purpose key off the wire is
// read. Every surface takes it from a request body or a public form where
// casing and stray whitespace are the caller's, and the key is then matched
// against consent_purpose.key — so two callers normalizing differently would
// resolve the same purpose to different rows, or to none.
//
// Spelled once because it was spelled twice: the delivery gate and the
// preference center each carried this line, and a third copy was about to be
// added for the preference center's duplicate-key check — where getting it
// wrong would have let a grant for "Newsletter " slip past the dedupe that
// makes a withdrawal win.
//
// Held by: TestThePurposeKeyIsNormalizedInExactlyOnePlace
// (backend/internal/modules/consent/purposekey_test.go)
func normalizedPurposeKey(key string) string {
	return strings.TrimSpace(strings.ToLower(key))
}
