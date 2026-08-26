// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import "testing"

// A routable field the fact lookup cannot resolve routes every lead on it to
// the empty string: `field` answers "" for a name it does not know, and a rule
// comparing that to a literal simply never matches. Nothing refuses the config
// — the editor accepts it, the engine reads it, and the leads it was meant to
// assign go to round-robin instead. The failure is silent at every layer.
//
// Compose already binds this vocabulary to the automation catalog's mirror
// (TestRoutableLeadFieldVocabularyIsSingleSourced). That holds the two LISTS
// against each other and says nothing about whether either one resolves, which
// is the half that decides where a lead lands.
//
// Each fact carries its own marker, so a `field` arm returning the wrong struct
// member fails here rather than reading as a pass.
func TestEveryRoutableLeadFieldResolvesToItsOwnFact(t *testing.T) {
	facts := leadRoutingFacts{
		Source:          "the source",
		CompanyName:     "the company name",
		CandidateOrgKey: "the candidate org key",
	}
	markers := map[string]bool{
		facts.Source: true, facts.CompanyName: true, facts.CandidateOrgKey: true,
	}
	if len(markers) != 3 {
		t.Fatal("the markers are no longer distinct, so a mixed-up arm would read as a pass")
	}
	seen := map[string]string{}
	for _, name := range RoutableLeadFields {
		got := facts.field(name)
		if got == "" {
			t.Errorf("RoutableLeadFields offers %q and leadRoutingFacts.field does not resolve it, "+
				"so every rule matching on that field compares the empty string and never fires",
				name)
			continue
		}
		if !markers[got] {
			t.Errorf("leadRoutingFacts.field(%q) returned %q, which is no fact this test seeded", name, got)
			continue
		}
		if other, taken := seen[got]; taken {
			t.Errorf("leadRoutingFacts.field resolves both %q and %q to the same fact %q, "+
				"so one of the two routes on a value that is not its own", other, name, got)
		}
		seen[got] = name
	}
	if len(seen) != len(markers) {
		t.Errorf("RoutableLeadFields reaches %d of the %d facts leadRoutingFacts carries — "+
			"a fact no field name resolves is one no rule can ever route on", len(seen), len(markers))
	}
}

// The inverse: an unknown name resolves to nothing rather than to whichever
// fact the switch happens to fall through to. This is what makes the empty
// string above a reliable signal instead of an accident of arm order.
func TestAnUnroutableFieldNameResolvesToNothing(t *testing.T) {
	facts := leadRoutingFacts{Source: "s", CompanyName: "c", CandidateOrgKey: "k"}
	if got := facts.field("owner_id"); got != "" {
		t.Errorf("leadRoutingFacts.field resolved the unroutable name %q to %q; routing is "+
			"lead-local by design and a name outside RoutableLeadFields must reach no fact",
			"owner_id", got)
	}
}
