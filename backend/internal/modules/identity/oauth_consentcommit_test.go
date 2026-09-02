// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestConsentScopesRefuseEmptyAndUnknown(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		// The screen disables Approve at zero, and this is the check behind
		// that button: a disabled control is not an authorization decision.
		// Unrefused, an empty set reaches mintPassport at TOKEN EXCHANGE and
		// fails in front of the CLIENT, with no way back to the human.
		{"empty", ""},
		{"whitespace only", "   "},
		{"outside the vocabulary", "read admin"},
		{"offline_access is not record authority", "read offline_access"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConsentedScopes(tc.raw)
			if !errors.Is(err, apperrors.ErrInvalidArgument) {
				t.Fatalf("parseConsentedScopes(%q) = %v, want ErrInvalidArgument", tc.raw, err)
			}
		})
	}
}

func TestConsentScopesKeepVocabularyOrder(t *testing.T) {
	// The human's tick order is not authority order, and the scopes travel
	// into an audit row and a passport that both read best in one order.
	got, err := parseConsentedScopes("send read enrich")
	if err != nil {
		t.Fatalf("a valid subset must parse: %v", err)
	}
	if want := []string{"read", "send", "enrich"}; !slices.Equal(got, want) {
		t.Fatalf("scopes = %v, want %v", got, want)
	}
}

func TestConsentScopesRefuseADuplicate(t *testing.T) {
	// A duplicate cannot come from the screen — it means a hand-built form or
	// a bug, and a passport carrying "read read" is a row nobody meant.
	if _, err := parseConsentedScopes("read read"); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("a repeated scope must refuse, got %v", err)
	}
}
