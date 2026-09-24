// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

//go:build !integration

package gates

// The routing form offers exactly the adapters and profiles the server accepts.
//
// `PROVIDERS` in `frontend/src/screens/ai-routing-fields.tsx` is a declared
// mirror of `ai.KnownProviders()`, and `PROFILES` in
// `frontend/src/screens/ai-routing.tsx` one of `ai.DeclaredProfiles()`, each
// compared in both directions: a name the form offers that the server refuses
// is a save rejected over a choice the reader picked from our own list, and
// one the server accepts that the form omits cannot be chosen from Settings.

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

// readFormConstant reads the quoted names of one `const NAME = [...] as const`.
func readFormConstant(t *testing.T, file, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	literal := regexp.MustCompile(`(?s)const ` + name + ` = \[(.*?)\] as const`).FindSubmatch(raw)
	if literal == nil {
		t.Fatalf("no `const %s = [...] as const` in %s — the mirror has gone blind", name, file)
	}
	var names []string
	for _, m := range quotedName.FindAllSubmatch(literal[1], -1) {
		names = append(names, string(m[1]))
	}
	if len(names) == 0 {
		t.Fatalf("%s in %s parsed as empty — the mirror has gone blind", name, file)
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
	for _, name := range offered {
		if !slices.Contains(served, name) {
			t.Errorf("the routing form offers provider %q, which SelectBrain does not serve — "+
				"saving it is refused over a name the reader picked from our own list", name)
		}
	}
	for _, name := range served {
		if !slices.Contains(offered, name) {
			t.Errorf("SelectBrain serves provider %q and the routing form's PROVIDERS omits it — "+
				"it cannot be bound from Settings", name)
		}
	}
}
