// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

// A provider name the contract cannot carry is refused where the adapter
// enters, not where a client validates a response.
//
// The name is the discriminator on every row a run touches, the provenance on
// the values it buys, and a field published under a pattern. Until this check
// existed, an adapter called `Acme-Data` registered, wrote rows, and was
// serialized into responses that violate the schema — a build that works
// locally and breaks for any client that checks, with the failure surfacing
// far from the author who chose the name.

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestARegisteredProviderIsNamedTheWayTheContractPublishesIt(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name      string
		registers bool
	}{
		{name: "surfe", registers: true},
		{name: "acme_data", registers: true},
		{name: "Acme-Data", registers: false},
		{name: "ACME", registers: false},
		{name: "2vendor", registers: false},
		{name: strings.Repeat("a", provider.NameMaxLength+1), registers: false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			// The real descriptor with ONLY the name changed: every other
			// field is one the registry already accepts, so a refusal here is
			// about the name and nothing else.
			_, err := NewRegistry(renamed{
				Adapter: NewOfflineProvider(0, time.Now), name: c.name,
			})
			switch {
			case c.registers && err != nil:
				t.Errorf("a provider named %q was refused: %v", c.name, err)
			case !c.registers && err == nil:
				t.Errorf("a provider named %q registered, and the contract publishes provider names as %s — "+
					"its rows and its responses would carry a value the schema refuses",
					c.name, provider.NamePattern)
			case !c.registers && !strings.Contains(err.Error(), provider.NamePattern):
				t.Errorf("the refusal of %q does not say what a legal name looks like: %v", c.name, err)
			}
		})
	}
}

// renamed is the shipped fake wearing a different name.
//
// A stub descriptor written here would prove the check runs against a shape
// nothing ships; this proves it runs against the real one, which is what an
// extension author's adapter will look like in every respect but its name.
type renamed struct {
	provider.Adapter
	name string
}

func (r renamed) Descriptor() provider.Descriptor {
	d := r.Adapter.Descriptor()
	d.Name = r.name
	return d
}
