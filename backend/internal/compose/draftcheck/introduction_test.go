// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The chips a first touch labels its own purpose with. Each is the rep
// introducing themselves or their company, which the prompt asks for, and none
// says anybody introduced anybody.
func TestTheSendersOwnIntroductionIsNotARelationshipClaim(t *testing.T) {
	for _, label := range []string{
		"Mich kurz vorstellen",
		"Mich kurz vorstellen und ein Gespräch anbieten",
		"Vorstellung von Nordlicht Software und Angebot für ein Gespräch",
		"introduce company",
		"introduce company and offer call",
		"introduce sender and offer a call",
		"introductory outreach",
	} {
		if findings := Reasoning([]string{label}, textlang.German, convstate.BandNone); len(findings) != 0 {
			t.Errorf("%q was refused: %+v", label, findings)
		}
	}
}

// A chip naming who made the introduction is still the claim the product
// cannot support, and the correction for it names the label and nothing else.
func TestADirectedIntroductionChipIsRefusedAsALabel(t *testing.T) {
	findings := Reasoning([]string{"introduced by Tobias"}, textlang.German, convstate.BandNone)
	if !hasRule(findings, RuleInventedRelationship) || !findings[0].InLabel {
		t.Fatalf("the directed chip should be an invented-relationship label finding, got %+v", findings)
	}
	if len(findings) != 1 {
		t.Errorf("one chip should be one finding, got %+v", findings)
	}
	feedback := Feedback(findings)
	if strings.Contains(feedback, "act of writing") {
		t.Errorf("a label-only correction must not tell the body to drop why it was written:\n%s", feedback)
	}
}

// The body states a direction nothing records, at every band and on a real
// thread too: the thread is where the reversed direction was read from.
func TestADirectedIntroductionInTheBodyIsRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		body    string
		lang    textlang.Lang
		refused bool
	}{
		"the reversed German claim": {"Hallo Marek,\n\nDass Romina den Kontakt hergestellt hat, freut mich sehr.", textlang.German, true},
		"put us in touch":           {"Hello Marek,\n\nTobias put us in touch last week.", textlang.English, true},
		"introduced us":             {"Hello Marek,\n\nRomina introduced us at the fair.", textlang.English, true},
		"the sender's own act":      {"Hello Marek,\n\nI would like to briefly introduce myself and Nordlicht Software.", textlang.English, false},
		"German self-introduction":  {"Guten Tag Marek Janetzke,\n\nich möchte mich kurz vorstellen.", textlang.German, false},
	} {
		t.Run(name, func(t *testing.T) {
			findings := Body(tc.body, tc.lang, convstate.BandFresh, Grounds{Threaded: true})
			if got := hasRule(findings, RuleInventedRelationship); got != tc.refused {
				t.Fatalf("refused = %v, want %v: %+v", got, tc.refused, findings)
			}
		})
	}
}

// After a long gap the prompt asks the draft to name what the exchange was
// about. A clause that does is the requested sentence, not an assumed memory;
// the bare gesture is still refused.
func TestNamingTheExchangeIsNotAnAssumedMemory(t *testing.T) {
	for opening, refused := range map[string]bool{
		"It has been eight months since we last spoke about the integration timeline.": false,
		"It has been some time since we last spoke regarding the integration project.": false,
		"It has been several months since we discussed the integration timeline.":      false,
		"As we discussed, the timeline depends on the budget round.":                   true,
		"It has been a while since we last spoke.":                                     true,
		"Since we last spoke a few weeks ago, has anything changed?":                   true,
		"I wanted to pick up the integration we discussed previously.":                 true,
		"Since we discussed it, has anything moved?":                                   true,
		"I would like to continue our conversation.":                                   true,
	} {
		findings := Body("Hello Priya,\n\n"+opening, textlang.English, convstate.BandMonths, Grounds{Threaded: true})
		if got := hasRule(findings, RuleAssumedMemory); got != refused {
			t.Errorf("%q: refused = %v, want %v: %+v", opening, got, refused, findings)
		}
	}
}
