// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The triage question put to a decision model. triageSystem is the same
// judgment in the LLM's form; the two are held together by tests rather than
// shared, because each model family needs its own shape:
// TestTriageCriteriaNameExactlyTheTriageKinds holds the labels,
// TestTriageDecisionStateCarriesTheLLMInputs the inputs, and certification the
// judgment.

import (
	"encoding/json"
	"slices"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// triageDecisionSite is the site the decision is asked at; it is the variant
// the certification case registers, so a certified row names this call.
const triageDecisionSite = "triage"

const triageDecisionInstructions = "What is this website, judged only from what `page.text` states (never from the domain in `page.url`)?"

var triageDecisionCriteria = map[string]string{
	siteKindCompany:  "A business, agency, institution or association offering something, including a solo consultancy that presents itself as a business.",
	siteKindPersonal: "One individual's own homepage, CV, portfolio or blog.",
	siteKindProvider: "A vendor's own site selling email mailboxes, web hosting or domains to the public.",
	siteKindParked:   "A registrar placeholder, coming-soon or under-construction page, error page, or domain-for-sale listing.",
	siteKindUnclear:  "The text does not say who or what this is, such as a bare sign-in page.",
}

type triageDecisionPage struct {
	URL  string `json:"url"`
	Text string `json:"text"`
}

type triageDecisionState struct {
	Page triageDecisionPage `json:"page"`
}

// triageDecision carries exactly the inputs triageRequest sends: the URL and
// the same excerpt of the same prose. A state that saw more than the LLM would
// certify a judgment the fallback cannot reproduce; one that saw less would be
// answering a different question.
func triageDecision(page crawlPage) decision.Request {
	state, _ := json.Marshal(triageDecisionState{Page: triageDecisionPage{ //nolint:errchkjson // two strings; marshal cannot fail
		URL:  page.URL,
		Text: triageExcerpt(page.prose()),
	}})
	return decision.Request{State: state, Questions: map[string]decision.Question{
		decisionQuestionKey: choiceQuestion(triageDecisionInstructions, triageDecisionCriteria),
	}}
}

// triageDecisionGate reads an answer at the LLM path's own floor. Two
// differences from that path are deliberate: the floor applies to EVERY label,
// and below it the answer falls back to the LLM instead of reading as "continue
// the crawl" — a decision model's unsure company is not evidence of anything.
func triageDecisionGate(a decision.Answer) ai.DecisionVerdict {
	if !slices.Contains(siteTriageKinds, a.Choice) {
		return ai.DecisionOffEnum
	}
	if a.Confidence < triageAbortConfidence {
		return ai.DecisionBelowFloor
	}
	return ai.DecisionAccepted
}

// readTriageDecision is an accepted answer as the site's own verdict. A
// decision carries no prose, so the reason stays empty and triageEvidence says
// only what the site classifies as.
func readTriageDecision(a decision.Answer) siteTriageVerdict {
	return siteTriageVerdict{Kind: a.Choice, Confidence: schema.Confidence(a.Confidence)}
}
