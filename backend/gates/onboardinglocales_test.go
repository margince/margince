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
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// onboardingLocaleEnums finds the onboarding locale enums in the contract by
// the varname prefix each declares, rather than by line number or by the
// enum's own text: a contract edit that moved either would otherwise take this
// gate's subject with it and leave the assertion passing over nothing.
var onboardingLocaleEnums = regexp.MustCompile(
	`enum: \[([^\]]*)\]\n[^\n]*\n?[^\n]*x-enum-varnames: \[(Onboarding(?:Company|Proposal)Locale[^\]]*)\]`)

func TestTheOnboardingConversationSpeaksEveryShippedLanguage(t *testing.T) {
	t.Parallel()

	contract, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	found := onboardingLocaleEnums.FindAllStringSubmatch(string(contract), -1)
	// Two: the message request's and the proposal parameter's. Pinned, because
	// a regex that stopped matching one of them would leave this gate green
	// over an enum nobody is checking.
	const declared = 2
	if len(found) != declared {
		t.Fatalf("found %d onboarding locale enum(s) in the contract, want %d — the pattern has gone "+
			"blind, and an enum this gate cannot see is one it cannot hold", len(found), declared)
	}

	want := make([]string, 0, len(textlang.Shipped))
	for _, lang := range textlang.Shipped {
		want = append(want, string(lang))
	}
	for _, match := range found {
		got := splitEnum(match[1])
		if !slices.Equal(got, want) {
			t.Errorf("the contract's %s enum admits %v, and the product ships %v.\n"+
				"A language in the shipped set that the enum refuses is one the frontend maps away "+
				"to English rather than 422s on — so the reader is onboarded in the wrong language "+
				"and nothing reports it. Widen the enum, its x-enum-varnames and the copy table "+
				"together.", strings.Split(match[2], ",")[0], got, want)
		}
	}

	// And the copy the conversation reads. Held here rather than in compose
	// because it is the same obligation: a language in the shipped set with no
	// copy behind it falls back to English, which is exactly the failure the
	// enum half exists to prevent, one layer in.
	for _, lang := range textlang.Shipped {
		if !onboardingCopyDeclares(t, string(lang)) {
			t.Errorf("the onboarding copy table has no entry for %q, which the product ships.\n"+
				"copyFor falls back to English, so this reader is onboarded in a language they did "+
				"not choose — silently. Add the entry in internal/compose/onboardingcopy.go.", lang)
		}
	}
}

// splitEnum reads a YAML flow sequence's members.
func splitEnum(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// onboardingCopyDeclares reports whether the copy table names this language.
//
// Read out of the source rather than by importing compose, which is what every
// gate in this package does with an unexported declaration: the table is
// package-private, and exporting it to be tested would widen an API for the
// benefit of the thing checking it.
func onboardingCopyDeclares(t *testing.T, code string) bool {
	t.Helper()
	body, err := os.ReadFile("internal/compose/onboardingcopy.go")
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
