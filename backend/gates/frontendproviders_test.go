// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

//go:build !integration

package gates

// The routing form offers the adapters and profiles the server accepts, and no
// others.
//
// `PROVIDERS` in `frontend/src/screens/ai-routing-fields.tsx` is a declared
// mirror of the provider registry's chat adapters, `DECISION_PROVIDERS` one of
// its decision adapters, and `PROFILES` in `frontend/src/screens/ai-routing.tsx`
// one of `ai.DeclaredProfiles()`, each compared in both directions: a name the
// form offers that the server refuses is a save rejected over a choice the
// reader picked from our own list, and one the server accepts that the form
// omits cannot be chosen from Settings.

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

const (
	routingFieldsFile = "../frontend/src/screens/ai-routing-fields.tsx"
	routingFormFile   = "../frontend/src/screens/ai-routing.tsx"
)

var quotedName = regexp.MustCompile(`"([^"]+)"`)

// readFormConstant returns the quoted names in the `const <name> = [...] as
// const` array of file. A missing or empty array fails rather than returning
// nothing, because an empty mirror agrees with nothing and would read as clean
// in one direction.
func readFormConstant(t *testing.T, file, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	block := regexp.MustCompile(`(?s)const ` + name + ` = \[(.*?)\] as const`).FindStringSubmatch(string(raw))
	if block == nil {
		t.Fatalf("no `const %s = [...] as const` in %s — the mirror has gone blind", name, file)
	}
	var names []string
	for _, m := range quotedName.FindAllStringSubmatch(block[1], -1) {
		names = append(names, m[1])
	}
	if len(names) == 0 {
		t.Fatalf("%s parsed empty in %s — the mirror has gone blind", name, file)
	}
	return names
}

func TestTheRoutingFormOffersExactlyTheProfilesTheServerAdmits(t *testing.T) {
	t.Parallel()
	offered := readFormConstant(t, routingFormFile, "PROFILES")
	var admitted []string
	for _, p := range ai.DeclaredProfiles() {
		admitted = append(admitted, string(p))
	}
	for _, name := range offered {
		if !slices.Contains(admitted, name) {
			t.Errorf("the routing form offers profile %q, which the server refuses on save", name)
		}
	}
	for _, name := range admitted {
		if !slices.Contains(offered, name) {
			t.Errorf("the server admits profile %q and the routing form's PROFILES omits it — "+
				"it cannot be chosen from Settings", name)
		}
	}
}

func TestTheRoutingFormOffersExactlyTheAdaptersTheServerServes(t *testing.T) {
	t.Parallel()
	offered := readFormConstant(t, routingFieldsFile, "PROVIDERS")
	served := ai.KnownProviders()
	for _, provider := range offered {
		if !slices.Contains(served, provider) {
			t.Errorf("the routing form offers provider %q, which SelectBrain does not serve — saving it is refused over a name the reader picked from our own list", provider)
		}
	}
	for _, provider := range served {
		if !slices.Contains(offered, provider) {
			t.Errorf("SelectBrain serves provider %q and the routing form's PROVIDERS omits it — it cannot be bound from Settings", provider)
		}
	}
}

// The decision model row offers only the adapters that answer a typed question
// with calibrated confidence. A completion adapter there would save a lane no
// decision site can call; a decision adapter missing from it cannot be bound.
func TestTheDecisionRowOffersExactlyTheDecisionProviders(t *testing.T) {
	t.Parallel()
	offered := readFormConstant(t, routingFieldsFile, "DECISION_PROVIDERS")
	served := ai.DecisionProviders()
	for _, provider := range offered {
		if !slices.Contains(served, provider) {
			t.Errorf("the decision model row offers provider %q, which the registry does not mark as a decision provider — saving it is refused over a name the reader picked from our own list", provider)
		}
	}
	for _, provider := range served {
		if !slices.Contains(offered, provider) {
			t.Errorf("the registry serves decision provider %q and the routing form's DECISION_PROVIDERS omits it — it cannot be bound from Settings", provider)
		}
	}
}
