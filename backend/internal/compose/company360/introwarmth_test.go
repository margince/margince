// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The claim the shape check cannot see. A cold route last used a year ago is
// not a friendship, and a colleague reads that as careless rather than as
// phrasing.
func TestAColdRoutesDraftMayNotClaimWarmth(t *testing.T) {
	t.Parallel()
	for name, draft := range map[string]string{
		"a friendship":   "Could you introduce me? You two are close friends.",
		"recency":        "You just spoke with her, so could you pass this along?",
		"a habit":        "Since you speak regularly, could you introduce me?",
		"shared history": "You go back years, so an introduction would help.",
	} {
		if claim := warmthOverclaim("cold", draft); claim == "" {
			t.Errorf("%s: a cold route's draft claiming %q was accepted", name, draft)
		}
	}
}

// The refusal has to name the claim, because that string is handed back to the
// model as the reason to try again. A refusal that says only "no" is answered
// with a synonym.
func TestTheRefusalNamesTheClaimItFound(t *testing.T) {
	t.Parallel()
	const draft = "You two are close friends and spoke recently."
	claim := warmthOverclaim("cold", draft)

	if claim == "" {
		t.Fatal("the draft was accepted")
	}
	if !strings.Contains(draft, claim) {
		t.Errorf("the refusal names %q, which is not in the draft", claim)
	}
}

// An ordinary ask has to pass. A floor that refuses prose claiming nothing
// teaches a rep the button is broken, and they stop using it.
func TestAnAskThatClaimsNothingPasses(t *testing.T) {
	t.Parallel()
	for name, draft := range map[string]string{
		"plain":                "Could you introduce me to Ute Sommer at Nordwind Logistik?",
		"a polite wish":        "I hope you are well. Could you introduce me to Ute Sommer?",
		"an ordinary deadline": "Before close of business, could you introduce me to Ute?",
		"a reason, not warmth": "You are the only route recorded to her, so could you introduce me?",
	} {
		if claim := warmthOverclaim("cold", draft); claim != "" {
			t.Errorf("%s: an honest ask was refused for claiming %q: %q", name, claim, draft)
		}
	}
}

// Only the cold band. A developing or strong route can say so and be right,
// and refusing it would cost a rep a usable draft while making nothing safer.
func TestAWarmerRouteMayDescribeItself(t *testing.T) {
	t.Parallel()
	const draft = "You two are close friends, so could you introduce me?"
	for _, band := range []string{"developing", "strong"} {
		if claim := warmthOverclaim(band, draft); claim != "" {
			t.Errorf("a %s route was refused for claiming %q", band, claim)
		}
	}
}

// claimsTheGraderReadsInContext are phrases the corpus lists and this floor
// does not hold, by choice.
//
// A grader reads them in a sentence and a substring match cannot. "well"
// appears in "I hope you are well" and "close" in "close of business". Holding
// them here would refuse ordinary prose, which costs more than the overclaim it
// would catch.
var claimsTheGraderReadsInContext = gatekit.Waive(map[string]string{
	"well":  `appears in "I hope you are well"`,
	"close": `appears in "close of business"`,
	"often": `appears in "as often happens"`,
})

// The corpus and this floor must not drift apart. A phrase the corpus scores
// and this floor neither holds nor excuses is an overclaim somebody thought
// worth grading, that nothing refuses.
func TestEveryPhraseTheCorpusScoresIsHeldOrExcused(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "aicert", "corpus", "draft_reply",
		"intro_cold_stale_route_01.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the corpus case this floor mirrors: %v", err)
	}
	var scenario struct {
		Expect struct {
			Answer []string `yaml:"answer"`
		} `yaml:"expect"`
	}
	if err := yaml.Unmarshal(raw, &scenario); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	if len(scenario.Expect.Answer) == 0 {
		t.Fatal("the corpus case lists no phrases, so this test asserts nothing")
	}
	// An exclusion the corpus has stopped listing is reported rather than left
	// standing: it would otherwise excuse a phrase nothing scores any more.
	defer claimsTheGraderReadsInContext.AssertAllMatched(t)

	for _, phrase := range scenario.Expect.Answer {
		folded := strings.ToLower(phrase)
		if claimsTheGraderReadsInContext.Waived(t, folded) {
			continue
		}
		var held bool
		for _, claim := range warmthClaims {
			if strings.Contains(claim, folded) || strings.Contains(folded, claim) {
				held = true
				break
			}
		}
		if !held {
			t.Errorf("the corpus scores %q and the floor neither holds nor excuses it", phrase)
		}
	}
}

// The floor has to hold where the draft is admitted. parseIntroDraft is the
// gate a model's reply passes, and ai.Ask re-asks on its refusal. A check
// placed anywhere else is one the served draft never meets.
func TestTheParseRefusesAColdRoutesOverclaim(t *testing.T) {
	t.Parallel()
	facts := introFacts{colleague: "Lars Bergmann", contact: "Ute Sommer", band: "cold"}
	const overclaiming = `{"subject":"An introduction to Ute Sommer",
	  "body":"Lars Bergmann, you and Ute Sommer are close friends, so could you introduce me?"}`

	if _, err := parseIntroDraft(overclaiming, facts); err == nil {
		t.Error("a cold route's draft claiming a friendship was admitted")
	} else if !strings.Contains(err.Error(), "close friend") {
		t.Errorf("the refusal is %q, which does not name the claim for the re-ask", err)
	}

	const honest = `{"subject":"An introduction to Ute Sommer",
	  "body":"Lars Bergmann, you are the only recorded route to Ute Sommer. Could you introduce me?"}`
	if _, err := parseIntroDraft(honest, facts); err != nil {
		t.Errorf("an honest ask was refused: %v", err)
	}
}
