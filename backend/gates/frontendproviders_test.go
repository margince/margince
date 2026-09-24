// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

//go:build !integration

package gates

// The routing form offers exactly the adapters SelectBrain serves.
//
// `PROVIDERS` in `frontend/src/screens/ai-routing-fields.tsx` is a declared
// mirror of `ai.KnownProviders()`, compared in both directions: a name the
// form offers that the server refuses is a save rejected over an adapter the
// reader picked from our own list, and an adapter the server serves that the
// form omits cannot be bound from Settings at all.

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

const routingFieldsFile = "../frontend/src/screens/ai-routing-fields.tsx"

var (
	providersLiteral = regexp.MustCompile(`(?s)const PROVIDERS = \[(.*?)\] as const`)
	quotedName       = regexp.MustCompile(`"([^"]+)"`)
)

func readFormProviders(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(routingFieldsFile)
	if err != nil {
		t.Fatalf("read %s: %v", routingFieldsFile, err)
	}
	literal := providersLiteral.FindSubmatch(raw)
	if literal == nil {
		t.Fatalf("no `const PROVIDERS = [...] as const` in %s — the mirror has gone blind", routingFieldsFile)
	}
	var names []string
	for _, m := range quotedName.FindAllSubmatch(literal[1], -1) {
		names = append(names, string(m[1]))
	}
	if len(names) == 0 {
		t.Fatalf("PROVIDERS in %s parsed as empty — the mirror has gone blind", routingFieldsFile)
	}
	return names
}

func TestTheRoutingFormOffersExactlyTheAdaptersTheServerServes(t *testing.T) {
	t.Parallel()
	offered := readFormProviders(t)
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
