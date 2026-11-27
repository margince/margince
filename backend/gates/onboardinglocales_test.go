// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The onboarding conversation speaks every language the product does.
//
// It used to speak two, in a shape that could not say so: each line of copy was
// an `if locale == "de"` with English underneath, so a third shipped language
// took the English branch silently. A Vietnamese reader was onboarded in
// English inside an otherwise Vietnamese product, and the only way to find that
// out was to run it.
//
// Three declarations of one set have to agree — textlang.Shipped, the
// contract's two onboarding locale enums, and the copy table the conversation
// reads. The frontend's own map is the fourth and holds itself: it is
// `satisfies Record<OnboardingLocale, true>`, so widening the enum stops the
// build until the map follows. This gate is the same obligation for the three
// that cannot state it in their own types.
//
// textlang.Shipped is the SUBJECT, not a party to the comparison: it is where
// the product says which languages it speaks, and the other two answer to it.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestTheOnboardingConversationSpeaksEveryShippedLanguage(t *testing.T) {
	t.Parallel()

	// TestEveryLanguageEnumInTheContractListsTheShippedLanguages holds the
	// contract's two onboarding enums. This holds the copy the conversation
	// reads. A shipped language with no copy falls back to English.
	for _, lang := range textlang.Shipped {
		if !onboardingCopyDeclares(t, string(lang)) {
			t.Errorf("the onboarding copy table has no entry for %q, which the product ships.\n"+
				"copyFor falls back to English, so this reader is onboarded in a language they did "+
				"not choose — silently. Add the entry in internal/compose/onboardingcopy.go.", lang)
		}
	}
}

// onboardingCopyDeclares reports whether the copy table names this language.
//
// Read out of the source rather than by importing compose, which is what every
// gate in this package does with an unexported declaration: the table is
// package-private, and exporting it to be tested would widen an API for the
// benefit of the thing checking it.
func onboardingCopyDeclares(t *testing.T, code string) bool {
	t.Helper()
	body, err := gatekit.ReadFile("internal/compose/onboardingcopy.go")
	if err != nil {
		t.Fatalf("reading the onboarding copy table: %v", err)
	}
	name := map[string]string{"en": "English", "de": "German", "vi": "Vietnamese"}[code]
	if name == "" {
		t.Fatalf("the product ships %q and this gate does not know its textlang constant — "+
			"teach it here rather than letting the copy check pass vacuously", code)
	}
	return strings.Contains(string(body), "textlang."+name+": {")
}
