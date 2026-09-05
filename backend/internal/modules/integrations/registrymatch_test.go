// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

// A descriptor's match rules are refused at registration when they name a
// field a person does not carry.
//
// This is the one failure mode of the mechanism that is otherwise silent: an
// unknown field is never present, so a rule carrying a typo is satisfied by
// nobody. Every subject is then skipped as unlookupable and the provider goes
// quiet — with no error anywhere, because refusing to spend money never looks
// like a fault.

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// matchingOn is the shipped fake with its match rules replaced, which is the
// shape an adapter author reaches by writing a rule by hand.
type matchingOn struct {
	provider.Adapter
	rules []provider.MatchRule
}

func (m matchingOn) Descriptor() provider.Descriptor {
	d := m.Adapter.Descriptor()
	d.MatchRules = m.rules
	return d
}

func TestAProviderCannotMatchOnAnIdentifierNobodyCarries(t *testing.T) {
	t.Parallel()

	// The shipped fake registers as it is, so the case below is not passing
	// over a descriptor that was refused for some other reason.
	shipped := NewOfflineProvider(0, time.Now).Descriptor()
	if len(shipped.MatchRules) == 0 {
		t.Fatal("the shipped fake declares no match rules, so it models a provider that looks everybody up " +
			"and no dev stack or test exercises the guard")
	}
	if _, err := NewRegistry(NewOfflineProvider(0, time.Now)); err != nil {
		t.Fatalf("the shipped adapter is refused by its own rule: %v", err)
	}

	for _, c := range []struct {
		name  string
		rules []provider.MatchRule
		names string
	}{
		{
			name:  "a field that does not exist",
			rules: []provider.MatchRule{{AllOf: []provider.IdentifierField{"middle_name"}}},
			names: "middle_name",
		},
		{
			name: "a typo in the any-of half",
			rules: []provider.MatchRule{{
				AllOf: []provider.IdentifierField{provider.IdentifierLastName},
				AnyOf: []provider.IdentifierField{"company_naem"},
			}},
			names: "company_naem",
		},
		{
			name:  "a rule naming nothing at all",
			rules: []provider.MatchRule{{}},
			names: "matches nobody",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewRegistry(matchingOn{Adapter: NewOfflineProvider(0, time.Now), rules: c.rules})
			if err == nil {
				t.Fatal("an adapter whose match rule can never be satisfied registered: every subject is " +
					"skipped as unlookupable and the provider goes silent with no error to notice")
			}
			if !strings.Contains(err.Error(), c.names) {
				t.Errorf("the refusal does not name what is wrong, so an author cannot act on it: %v", err)
			}
		})
	}
}

// An adapter declaring no rules at all is allowed: that is how a provider says
// it can look anybody up, and it is what every adapter written before this
// mechanism existed declares by omission.
func TestAProviderMayDeclareNoMatchRules(t *testing.T) {
	t.Parallel()
	if _, err := NewRegistry(matchingOn{Adapter: NewOfflineProvider(0, time.Now), rules: nil}); err != nil {
		t.Errorf("an adapter declaring no match rules was refused, so adding this field broke every "+
			"provider that does not use it: %v", err)
	}
}

// requiresChain is the shipped fake with a prerequisite graph replaced, which
// is the shape an adapter author reaches by declaring one by hand.
type requiresChain struct {
	provider.Adapter
	requires map[provider.Category]provider.Category
}

func (r requiresChain) Descriptor() provider.Descriptor {
	d := r.Adapter.Descriptor()
	d.RequiresAnswerTo = r.requires
	return d
}

// A prerequisite chain or cycle is refused where the author declares it.
//
// Every reader of RequiresAnswerTo walks ONE hop: the price of a purchase, the
// set the button sends, the check that refuses a lone request, the free-set
// derivation. Over a chain that is wrong in the direction that costs money —
// A's button quotes and sends A+B, and the server refuses it for missing C —
// and a cycle has no ordering that satisfies "answer first" at all.
func TestAPrerequisiteChainIsRefusedAtRegistration(t *testing.T) {
	t.Parallel()

	// The shipped fake registers as it is, so the cases below are not passing
	// over a descriptor refused for some other reason.
	shipped := NewOfflineProvider(0, time.Now).Descriptor()
	if len(shipped.RequiresAnswerTo) == 0 {
		t.Fatal("the shipped fake declares no prerequisite, so no dev stack or test exercises this rule")
	}
	if _, err := NewRegistry(NewOfflineProvider(0, time.Now)); err != nil {
		t.Fatalf("the shipped adapter is refused by its own rule: %v", err)
	}

	for _, c := range []struct {
		name     string
		requires map[provider.Category]provider.Category
		names    string
	}{
		{
			name: "a chain",
			requires: map[provider.Category]provider.Category{
				"mobile":             "professional_email",
				"professional_email": "linkedin_profile",
			},
			names: "linkedin_profile",
		},
		{
			name: "a two-step cycle",
			requires: map[provider.Category]provider.Category{
				"mobile":             "professional_email",
				"professional_email": "mobile",
			},
			names: "professional_email",
		},
		{
			name:     "a category needing itself",
			requires: map[provider.Category]provider.Category{"mobile": "mobile"},
			names:    "its own prerequisite",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewRegistry(requiresChain{
				Adapter: NewOfflineProvider(0, time.Now), requires: c.requires,
			})
			if err == nil {
				t.Fatal("an adapter declaring a prerequisite graph deeper than one hop registered: every " +
					"reader walks one hop, so its purchases are priced and requested short")
			}
			if !strings.Contains(err.Error(), c.names) {
				t.Errorf("the refusal does not name what is wrong, so an author cannot act on it: %v", err)
			}
		})
	}
}

// cascadingTo replaces a descriptor's cascades, so a case can declare the graph
// an author would write by hand.
type cascadingTo struct {
	provider.Adapter
	cascades []provider.Cascade
}

func (c cascadingTo) Descriptor() provider.Descriptor {
	d := c.Adapter.Descriptor()
	d.Cascades = c.cascades
	return d
}

// Two cascades for one fallback are refused where the author declares them.
//
// The catalog prices a fallback with EVERY trigger that can fire it and names
// ONE, because a buy button asks for a pair. Declared twice, the button quotes
// the price of both triggers and posts one, and the server refuses the press
// for the trigger left out — a number on a button that nobody can spend.
func TestTwoCascadesForOneFallbackAreRefusedAtRegistration(t *testing.T) {
	t.Parallel()

	// The shipped fake registers as it is, so the case below is not passing
	// over a descriptor refused for some other reason.
	shipped := NewOfflineProvider(0, time.Now).Descriptor()
	if len(shipped.Cascades) == 0 {
		t.Fatal("the shipped fake declares no cascade, so no dev stack or test exercises this rule")
	}
	if _, err := NewRegistry(NewOfflineProvider(0, time.Now)); err != nil {
		t.Fatalf("the shipped adapter is refused by its own rule: %v", err)
	}

	_, err := NewRegistry(cascadingTo{
		Adapter: NewOfflineProvider(0, time.Now),
		cascades: []provider.Cascade{
			{Category: "personal_email", After: "professional_email", Cost: map[provider.Pool]int{"email": 2}},
			{Category: "personal_email", After: "linkedin_profile", Cost: map[provider.Pool]int{"email": 2}},
		},
	})
	if err == nil {
		t.Fatal("an adapter declaring one fallback after two triggers registered: the catalog would price " +
			"both and name one, and every press of that button is refused")
	}
	if !strings.Contains(err.Error(), "linkedin_profile") {
		t.Errorf("the refusal does not name the second trigger, so an author cannot act on it: %v", err)
	}
}
