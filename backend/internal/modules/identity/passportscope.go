// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The passport scope vocabulary, and the two different ways a mint's scopes
// can be wrong.

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// passportScopeList renders the vocabulary for a refusal, in the order
// passportScopeVocabulary declares.
//
// Derived rather than written out, because a hand-typed copy drifts from the
// list it copies.
//
// A scope added to the vocabulary would be grantable while every refusal still
// named the old five.
func passportScopeList() string {
	names := make([]string, 0, len(passportScopeVocabulary))
	for _, sc := range passportScopeVocabulary {
		names = append(names, string(sc))
	}
	return strings.Join(names, "|")
}

// admitScopes refuses the two scope faults a mint can carry, which are
// different mistakes and read as different sentences.
//
// No scope at all is a missing field: the contract declares `scopes` required
// with `minItems` 1.
//
// Answered as a vocabulary fault, it told a caller who sent no scope that
// their scope was not in the list.
func admitScopes(scopes []string) error {
	if len(scopes) == 0 {
		return &InvalidPassportFieldError{
			Field: fieldScopes, Code: "required",
			Message: "name at least one scope: " + passportScopeList(),
		}
	}
	for _, sc := range scopes {
		if !validScopes[principal.Scope(sc)] {
			return &InvalidScopeError{Scope: sc}
		}
	}
	return nil
}

// InvalidScopeError maps to 422. It answers a scope outside the vocabulary,
// while naming no scope at all is a missing field and answered as one.
type InvalidScopeError struct{ Scope string }

func (e *InvalidScopeError) Error() string {
	return "scope " + e.Scope + " is not one of " + passportScopeList()
}

// fieldScopes is the request field every scope refusal names.
const fieldScopes = "scopes"
