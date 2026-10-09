// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// A passport mint refuses two different scope faults, and one sentence
// answered both.
//
// Naming no scope at all was reported as naming a scope outside the
// vocabulary.

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestNamingNoScopeIsAMissingFieldNotAnUnknownValue(t *testing.T) {
	t.Parallel()
	// Absent, empty and null all arrive here as an empty slice.
	err := admitScopes([]string{})

	fault, ok := errors.AsType[*InvalidPassportFieldError](err)
	if !ok {
		t.Fatalf("err = %v, want InvalidPassportFieldError — no scope is a missing field", err)
	}
	if fault.Field != fieldScopes {
		t.Errorf("field = %q, want %q", fault.Field, fieldScopes)
	}
	if fault.Code != "required" {
		t.Errorf("code = %q, want required — the caller sent no scope, not a wrong one", fault.Code)
	}
	if strings.Contains(fault.Message, "(none)") {
		t.Error("the sentence still reports a scope called (none)")
	}
}

// A scope outside the vocabulary keeps the vocabulary sentence, so the fix
// separates the two faults rather than replacing one with the other.
func TestAnUnknownScopeStillNamesTheVocabulary(t *testing.T) {
	t.Parallel()
	err := (&InvalidScopeError{Scope: "bogus"}).Error()
	if !strings.Contains(err, "is not one of") {
		t.Errorf("sentence = %q, want the vocabulary refusal", err)
	}
	for _, sc := range passportScopeVocabulary {
		if !strings.Contains(err, string(sc)) {
			t.Errorf("sentence = %q, missing the declared scope %q", err, sc)
		}
	}
}

// The rendered list is derived from the declared vocabulary, so a scope added
// there appears in every refusal without a sentence being edited.
func TestTheScopeListIsDerivedFromTheVocabulary(t *testing.T) {
	t.Parallel()
	got := passportScopeList()
	if n := len(strings.Split(got, "|")); n != len(passportScopeVocabulary) {
		t.Errorf("list = %q has %d entries, want %d", got, n, len(passportScopeVocabulary))
	}
	// Order carries meaning: the vocabulary ascends in authority, and a refusal
	// that reorders it reads as a different set.
	if want := string(principal.ScopeRead); !strings.HasPrefix(got, want) {
		t.Errorf("list = %q, want it to open with %q", got, want)
	}
}
