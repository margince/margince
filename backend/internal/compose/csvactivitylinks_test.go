// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
)

func TestLinksParseTypeKeyPairs(t *testing.T) {
	keys, why := parseLinks("deal:D1; contact:jo@acme.com ;Company:acme.com;deal:D1;;")
	want := []linkKey{
		{Object: "deal", Key: "D1"}, {Object: "contact", Key: "jo@acme.com"}, {Object: "company", Key: "acme.com"},
	}
	if why != nil || !slices.Equal(keys, want) {
		t.Fatalf("keys = %v, why = %v; want %v with the repeat and the empty parts dropped", keys, why, want)
	}
	if keys, why := parseLinks("deal:Q3: big one"); why != nil || len(keys) != 1 || keys[0].Key != "Q3: big one" {
		t.Fatalf("a key holding a colon = %v, %v; want the whole key kept", keys, why)
	}
	for cell, code := range map[string]migration.ReasonCode{
		"deal D1":    migration.ReasonLinkMalformed,
		"deal:":      migration.ReasonLinkMalformed,
		":D1":        migration.ReasonLinkMalformed,
		"project:P1": migration.ReasonLinkTypeUnknown,
		"owner:jo@x": migration.ReasonLinkTypeUnknown,
	} {
		if _, why := parseLinks(cell); why == nil || why.Code != code {
			t.Errorf("%q = %v, want %s", cell, why, code)
		}
	}
	if keys, why := parseLinks(""); keys != nil || why != nil {
		t.Errorf("an empty cell = %v, %v; want no pairs and no problem", keys, why)
	}
}

func TestADealLinkIsNeverResolvedAsACompanyName(t *testing.T) {
	// A resolver with no pool: a company-name lookup would panic on it, so an
	// answer proves the step went nowhere near the company table.
	r := &linkResolver{}
	for _, object := range []string{migration.ObjectDeal, migration.ObjectLead} {
		answer, err := r.naturalKeyOf(context.Background(), object, "Acme")
		if err != nil || answer.Found || answer.Pending || answer.Why.Code != "" {
			t.Errorf("%s:Acme = %+v, %v; want no answer, never the company named Acme", object, answer, err)
		}
	}
}
