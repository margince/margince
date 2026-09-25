// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

//go:build !integration

package gates

// The routing form offers an admin an adapter for every tier, and the frontend
// cannot read Go, so its PROVIDERS list is a declared mirror of the server's
// provider registry. Both directions fail: a name the form offers that
// SelectBrain does not serve is a save refused for a choice the form made, and
// an adapter SelectBrain serves that the form omits is a binding nobody can
// choose from Settings.

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

const routingFieldsFile = "../frontend/src/screens/ai-routing-fields.tsx"

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
