// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

import (
	"errors"
	"fmt"
	"sort"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// ErrUnknownProvider reports a provider name this build does not carry. The
// registered set is closed (PI-PARAM-1): a name absent here is a 404, never a
// call to something we have not reviewed.
var ErrUnknownProvider = errors.New("integrations: unknown provider")

// Registry is the closed set of adapters compiled into this build. Compose
// decides what goes in it — the real Surfe adapter, or the deterministic fake
// for a dev stack and the test lane.
//
// An empty registry is a supported configuration, not a broken one: with no
// adapter registered, every domain surface renders "no provider connected"
// and nothing can reach the network (PI-AC-9).
type Registry struct {
	adapters map[string]provider.Adapter
}

// NewRegistry builds a registry from the adapters compose supplies. It
// refuses a descriptor whose billing basis the platform cannot honour: the
// alternative is discovering at reservation time that we cannot price a run
// we have already admitted.
func NewRegistry(adapters ...provider.Adapter) (*Registry, error) {
	r := &Registry{adapters: map[string]provider.Adapter{}}
	for _, a := range adapters {
		d := a.Descriptor()
		// The name is the discriminator on every row this provider's runs
		// touch, the provenance on the values they buy, and a field the API
		// publishes under a pattern. An adapter refused here is one whose
		// author finds out now, rather than through a client that validates
		// responses and reports a schema breach far from the cause.
		//
		// No name at all goes through the same door: it is one of the names
		// the contract cannot carry, and a caller who gets a different
		// sentence for it has to be told the rule twice.
		if !provider.ValidName(d.Name) {
			return nil, fmt.Errorf(
				"integrations: provider name %q is not one the contract can carry (%s, at most %d characters): "+
					"lower-case letters, digits and underscores, starting with a letter",
				d.Name, provider.NamePattern, provider.NameMaxLength)
		}
		if _, dup := r.adapters[d.Name]; dup {
			return nil, fmt.Errorf("integrations: provider %q registered twice", d.Name)
		}
		if _, err := d.WorstCase(d.Categories); err != nil {
			return nil, fmt.Errorf("integrations: provider %q cannot be registered: %w", d.Name, err)
		}
		for _, category := range d.Categories {
			if _, priced := d.CostTable[category]; !priced {
				// A missing entry reads as free everywhere the platform asks
				// what something costs, so an omission is a category bought
				// automatically and reserved at nothing. An empty map is how
				// an adapter says "this one really is free" — the difference
				// between deciding and forgetting.
				return nil, fmt.Errorf("integrations: provider %q declares category %q with no cost entry: price it, or declare it free with an empty one", d.Name, category)
			}
		}
		if err := pricesOnlyDeclaredPools(d); err != nil {
			return nil, err
		}
		if d.EgressHost == "" {
			return nil, fmt.Errorf("integrations: provider %q declared no egress host", d.Name)
		}
		r.adapters[d.Name] = a
	}
	return r, nil
}

// pricesOnlyDeclaredPools refuses an adapter whose cost table names a pool it
// never declared.
//
// The reservation is keyed by the pools CostTable produces and the settlement
// is keyed by the pools the adapter reports having spent. Nothing makes those
// two vocabularies agree, and a mismatch is silent in the direction that costs
// money: reconcile finds no figure for a held pool and, on per-successful-result
// billing, releases it — so the customer's monthly ceiling is credited back a
// charge the vendor kept, and later runs spend it twice.
//
// CreditPools is the adapter's own statement of what it bills in, so it is the
// declaration to hold both sides to. Checked at registration, where an author
// finds out immediately rather than through a ledger that quietly drifts.
func pricesOnlyDeclaredPools(d provider.Descriptor) error {
	declared := make(map[provider.Pool]bool, len(d.CreditPools))
	for _, pool := range d.CreditPools {
		declared[pool] = true
	}
	for category, cost := range d.CostTable {
		for pool := range cost {
			if !declared[pool] {
				return fmt.Errorf(
					"integrations: provider %q prices category %q in pool %q, which it does not declare in CreditPools: "+
						"a hold taken in an undeclared pool cannot be matched to what the vendor reports spending, and "+
						"settles as a refund of a charge they kept",
					d.Name, category, pool)
			}
		}
	}
	return nil
}

// Names returns the registered providers in a stable order, so a settings
// card does not reshuffle between reads.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.adapters))
	for n := range r.adapters {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Adapter resolves one provider.
func (r *Registry) Adapter(name string) (provider.Adapter, error) {
	a, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, name)
	}
	return a, nil
}

// Descriptor resolves one provider's static declaration.
func (r *Registry) Descriptor(name string) (provider.Descriptor, error) {
	a, err := r.Adapter(name)
	if err != nil {
		return provider.Descriptor{}, err
	}
	return a.Descriptor(), nil
}

// Empty reports whether anything is registered at all.
func (r *Registry) Empty() bool { return len(r.adapters) == 0 }
