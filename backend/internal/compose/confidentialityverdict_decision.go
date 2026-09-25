// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The thread question put to a decision model. confidentialitySystem is the
// same judgment in the LLM's form; the two are held together by tests rather
// than shared, because each model family needs its own shape:
// TestConfidentialityCriteriaNameExactlyTheThreadKinds holds the labels,
// TestConfidentialityDecisionStateCarriesTheLLMInputs the inputs, and
// certification the judgment.
//
// The task is local_only, so the lane answers here only when the deployment's
// decision model is local; everywhere else the router skips it and the ladder
// answers as before.

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// confidentialityDecisionSite is the site the decision is asked at; it is the
// variant the certification case registers, so a certified row names this call.
const confidentialityDecisionSite = "thread"

const confidentialityDecisionInstructions = "May the mailbox owner's colleagues read `thread`? Which kind of thread is it?"

var confidentialityDecisionCriteria = map[string]string{
	confidentialityOrdinary:      "Routine company business: sales, delivery, support, suppliers, scheduling, and invoices or expenses for the company's own trade (including a work trip or conference paid personally). Saying an NDA exists or is signed is still ordinary.",
	confidentialityLegal:         "A dispute, a claim, a contract under negotiation, or correspondence with lawyers, including counsel's fees.",
	confidentialityFinancial:     "The company's own corporate finance: shareholders, funding, valuation, tax, audit, banking, an acquisition.",
	confidentialityPersonnel:     "A named individual as employee or candidate: salary, employment contract, termination, grievance, performance, application.",
	confidentialityPersonal:      "The mailbox owner's private life: family, health, their home, rent, phone or utility bill, personal bank or card alert, private subscription, even if forwarded for reimbursement.",
	confidentialitySecurity:      "A breach, intrusion, leaked credentials, or an unpatched vulnerability under embargo.",
	confidentialityExplicitlyMar: "The message itself asks for confidence: marked confidential or vertraulich, or asks to keep it to a small circle or not forward it.",
}

type confidentialityDecisionThread struct {
	Subject string `json:"subject"`
	// Attachments is an empty list rather than absent when there are none,
	// as the LLM request says "Attachments:" with nothing after it.
	Attachments []string `json:"attachments"`
	Body        string   `json:"body"`
}

type confidentialityDecisionState struct {
	Thread confidentialityDecisionThread `json:"thread"`
}

// confidentialityDecision carries exactly the inputs confidentialityRequest
// sends: the subject, every attachment name and the body.
func confidentialityDecision(row capture.PendingThread) decision.Request {
	attachments := row.Attachments
	if attachments == nil {
		attachments = []string{}
	}
	state, _ := json.Marshal(confidentialityDecisionState{Thread: confidentialityDecisionThread{ //nolint:errchkjson // strings only; marshal cannot fail
		Subject:     row.Subject,
		Attachments: attachments,
		Body:        row.Body,
	}})
	return decision.Request{State: state, Questions: map[string]decision.Question{
		decisionQuestionKey: choiceQuestion(confidentialityDecisionInstructions, confidentialityDecisionCriteria),
	}}
}

// confidentialityDecisionGate reads an answer at the site's one floor, and
// applies it to EVERY label where the LLM path applies it to the opening one
// alone. The difference is deliberate: below the floor the LLM path still
// holds, while a decision below it falls back to the LLM — so an unsure hold
// is asked again rather than kept, and the LLM's own reading decides.
func confidentialityDecisionGate(a decision.Answer) ai.DecisionVerdict {
	if _, known := statusForConfidentiality(a.Choice); !known {
		return ai.DecisionOffEnum
	}
	if a.Confidence < confidentialityFloor {
		return ai.DecisionBelowFloor
	}
	return ai.DecisionAccepted
}

// readConfidentialityDecision is an accepted answer as the engine's own
// result, about the one thread that was asked.
func readConfidentialityDecision(row capture.PendingThread, a decision.Answer) confidentialityResult {
	return confidentialityResult{ID: row.ID.String(), Verdict: a.Choice, Confidence: schema.Confidence(a.Confidence)}
}
