// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// A row this run skipped is not in the CRM, so a link to it follows the run's
// broken-link policy and says the row was skipped, not that it never existed.
func TestALinkToARowThisRunSkippedFollowsTheBrokenLinkPolicy(t *testing.T) {
	contactID := ids.NewV7()
	row := migration.Row{ExternalID: "A-1", Line: 4, Fields: map[string]any{
		activityTargetLinks: "deal:D-1;contact:C-1", activityTargetType: "note",
	}}
	cases := map[string]struct {
		policy string
		skip   bool
		code   migration.ReasonCode
	}{
		"default skips the row":  {"", true, migration.ReasonLinkRowSkipped},
		"drop keeps other links": {string(crmcontracts.ImportOnBrokenLinkDropLink), false, migration.ReasonLinkRowSkippedDropped},
	}
	for name, c := range cases {
		links := newTestLinkResolver(t)
		links.markLanded(migration.ObjectContact, "C-1", contactID)
		links.markRefused(migration.ObjectDeal, "D-1")
		w := &csvWriters{
			object: migration.ObjectActivity, links: links, onBrokenLink: c.policy,
			bindings: map[string]migration.Binding{}, nativeIDs: map[string]ids.UUID{},
			preset: presetSet{checked: true},
		}
		got, err := w.activityLinkProblemOf(context.Background(), row)
		if err != nil || got.problem == nil {
			t.Fatalf("%s: got %+v err %v, want a problem", name, got, err)
		}
		if got.skip != c.skip || got.problem.Code != c.code {
			t.Errorf("%s: skip=%v code=%q, want skip=%v code=%q", name, got.skip, got.problem.Code, c.skip, c.code)
		}
		if c.skip {
			continue
		}
		kept, ok := got.links[activityTargetLinks].([]activities.ActivityLinkInput)
		if !ok || len(kept) != 1 || kept[0].EntityID != contactID {
			t.Errorf("%s: links = %+v, want only the contact", name, got.links)
		}
	}
}
