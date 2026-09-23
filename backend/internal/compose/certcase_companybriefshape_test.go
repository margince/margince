// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

// prepareCompanyAsk prepares the ask site over the same account fixture the
// brief tests use, plus the question that site needs.
func prepareCompanyAsk(t *testing.T, expected string) aitasks.PreparedCase {
	t.Helper()
	fixture := strings.TrimSuffix(strings.TrimSpace(companyBriefFixtureJSON), "}") +
		`, "question": "whats_open"}`
	prepared, err := companyAskCases{}.Prepare(json.RawMessage(fixture), json.RawMessage(expected))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	return prepared
}

// The company_brief case reads the shape company_brief asks for.
//
// companyBriefCase serves TWO sites with two different wire shapes.
// summarize/company_ask asks for {"sentences":[...]} (companybrief.AskRequest)
// and summarize/company_brief asks for {"sections":[{"kind":..,"sentences":..}]}
// (companybrief.BriefRequest, and briefSystem spells it out). Evaluate parsed
// both with ParseBrief, which reads `sentences` only.
//
// Unmarshalling a sections document into a struct that names only `sentences`
// does not fail — it succeeds with nil — so every company_brief run reported
// "no sentence cited a record of this account" and abstained. Not sometimes:
// ALWAYS, for every model, whatever it wrote. Both of this site's scenarios
// were unpassable, and Gemma 4 and Gemini-3.1-flash-lite abstaining on them
// with the same message read as two models agreeing rather than as a harness
// that could not see their answers.
//
// The request shape and the parse shape are chosen together now, because that
// is the pairing that went wrong.
func TestTheCompanyBriefCaseReadsTheSectionedShapeItAsksFor(t *testing.T) {
	prepared := prepareCompanyBrief(t, `["stalled_retrofit"]`)
	sent := requestIDs(t, prepared)
	dealID := firstRecordID(t, sent)

	// The sectioned shape briefSystem demands, citing the record the scenario
	// says the brief is about.
	sectioned := `{"sections":[{"kind":"activity","sentences":[` +
		`{"text":"The retrofit has stalled.","nature":"fact",` +
		`"evidence":[{"entity_type":"deal","entity_id":"` + dealID + `"}]}]}]}`

	got := prepared.Evaluate(aitasks.Trace{Output: sectioned})
	if got.Result != aitasks.OutcomeAccepted {
		t.Fatalf("Evaluate = %q (%s), want %q — the case cannot read the shape its own request asks for",
			got.Result, got.Detail, aitasks.OutcomeAccepted)
	}
}

// And the site that really does answer in sentences still does.
//
// Held separately because the fix is a per-site parser: getting company_brief
// right by changing the shared parser would break company_ask, and a test that
// only covered one would not notice.
func TestTheCompanyAskCaseStillReadsFlatSentences(t *testing.T) {
	prepared := prepareCompanyAsk(t, `["stalled_retrofit"]`)
	sent := requestIDs(t, prepared)
	dealID := firstRecordID(t, sent)

	flat := `{"sentences":[{"text":"The retrofit has stalled.","nature":"fact",` +
		`"evidence":[{"entity_type":"deal","entity_id":"` + dealID + `"}]}]}`

	got := prepared.Evaluate(aitasks.Trace{Output: flat})
	if got.Result != aitasks.OutcomeAccepted {
		t.Fatalf("Evaluate = %q (%s), want %q", got.Result, got.Detail, aitasks.OutcomeAccepted)
	}
}

// A sentence about the account itself survives, which is the whole point of the
// account carrying its id.
//
// Asserted on the ACCOUNT's id and on a sentence citing it, not on the presence
// of any `"id":` in the prompt: this fixture carries an open deal, whose record
// id satisfies that weaker check on its own. Written the weak way first, it
// passed with the account id absent — the regression it names would have walked
// straight back in under a green test.
func TestACompanyBriefSentenceAboutTheAccountItselfSurvives(t *testing.T) {
	prepared := prepareCompanyBrief(t, `["stalled_retrofit"]`)
	sent := requestIDs(t, prepared)

	// Input serializes the account's own id first and its name next, so this
	// pair identifies the SUBJECT rather than one of its records.
	account := regexp.MustCompile(`\{"id":"([0-9a-f-]{36})","name":`).FindStringSubmatch(sent)
	if account == nil {
		t.Fatalf("the summary carries no account id before its name, so no sentence about the "+
			"account can cite it:\n%s", sent)
	}

	sectioned := `{"sections":[{"kind":"snapshot","sentences":[` +
		`{"text":"They make automotive parts.","nature":"fact",` +
		`"evidence":[{"entity_type":"company","entity_id":"` + account[1] + `"}]}]}]}`

	if got := prepared.Evaluate(aitasks.Trace{Output: sectioned}); got.Result == aitasks.OutcomeAbstained {
		t.Errorf("a sentence citing the account by the id the summary supplied was dropped: %s", got.Detail)
	}
}
