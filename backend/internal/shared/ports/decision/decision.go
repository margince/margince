// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package decision is the Tier-0 vocabulary for asking a decision model: a
// structured state, the typed questions asked of it, and the calibrated answer
// each one gets back.
//
// It lives at Tier-0 because the agreement crosses a module boundary: the ai
// module sends the call and the composition layer's site adapters build the
// request and read the answer, and a module never imports a sibling.
package decision

import (
	"context"
	"encoding/json"
)

// Request is one decision call: a structured state and the typed questions
// asked of it. The state is data and the questions are instructions; the wire
// keeps them apart, so no fence is needed.
type Request struct {
	Model     string
	State     json.RawMessage
	Questions map[string]Question
}

// QuestionType is how a question is answered.
type QuestionType string

// Choice is the only question type a site asks today: pick one label from a
// closed set. Other types wait for their first adopter.
const Choice QuestionType = "choice"

// Question is one typed question. Criteria maps each label to the rule that
// makes THAT label right, so the model is told what distinguishes the labels
// rather than only their names.
type Question struct {
	Type         QuestionType
	Instructions string
	Criteria     map[string]string
}

// Answer is the model's answer to one question. Confidence is how peaked the
// distribution over labels is, as the wire computes it; a site reads it
// against its own floor, never as a probability of being right.
type Answer struct {
	Choice        string
	Confidence    float64
	Probabilities map[string]float64
}

// Response is one decision call's answers, keyed by question, and what served
// them. ServedModel and ServedProvider are empty when the wire does not say.
type Response struct {
	Answers        map[string]Answer
	InputTokens    int
	ServedModel    string
	ServedProvider string
}

// Client sends a decision request to one endpoint.
type Client interface {
	Decide(ctx context.Context, req Request) (Response, error)
}
