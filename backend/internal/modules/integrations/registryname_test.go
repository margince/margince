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
		{name: "vendor2", registers: true},
		{name: strings.Repeat("a", provider.NameMaxLength), registers: true},
		{name: "", registers: false},
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

// An adapter that prices a category in a pool it never declared is refused
// where it enters, because the mismatch is silent and costs money.
//
// The reservation is keyed by the pools the cost table produces; the settlement
// is keyed by the pools the adapter reports spending. Nothing else makes those
// agree — and a hold in a pool the vendor never reports settles, on
// per-successful-result billing, as a REFUND of a charge they kept. The
// customer's monthly ceiling is credited back money that left, and later runs
// spend it a second time.
func TestAProviderCannotPriceInAPoolItNeverDeclared(t *testing.T) {
	t.Parallel()
	shipped := NewOfflineProvider(0, time.Now).Descriptor()
	if len(shipped.CreditPools) == 0 || len(shipped.CostTable) == 0 {
		t.Fatal("the shipped fake declares no pools or no prices, so this case would pass over an empty subject")
	}

	if _, err := NewRegistry(NewOfflineProvider(0, time.Now)); err != nil {
		t.Fatalf("the shipped adapter is refused by its own rule: %v", err)
	}

	_, err := NewRegistry(pricedInAnUndeclaredPool{Adapter: NewOfflineProvider(0, time.Now)})
	if err == nil {
		t.Fatal("an adapter pricing a category in an undeclared pool registered: its holds settle as refunds " +
			"of charges the vendor kept, and nothing downstream can notice")
	}
	if !strings.Contains(err.Error(), "ghost_pool") {
		t.Errorf("the refusal does not name the offending pool, so an author cannot act on it: %v", err)
	}
}

// pricedInAnUndeclaredPool is the shipped fake billing one category in a pool
// missing from its own CreditPools — the shape an extension author reaches by
// adding a price and forgetting the declaration.
type pricedInAnUndeclaredPool struct {
	provider.Adapter
}

func (p pricedInAnUndeclaredPool) Descriptor() provider.Descriptor {
	d := p.Adapter.Descriptor()
	priced := make(map[provider.Category]map[provider.Pool]int, len(d.CostTable))
	for category, cost := range d.CostTable {
		priced[category] = cost
	}
	priced[d.Categories[0]] = map[provider.Pool]int{"ghost_pool": 1}
	d.CostTable = priced
	return d
}
