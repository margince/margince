// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesassessment

// The request sends the shape Parse reads, closed to the refs it showed. A
// constrained decoder cannot write a reply Parse refuses whole, and the schema
// admits every reply Parse keeps.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/schema"
)

func schemaInput() Input {
	in := assessmentInput()
	in.Actions = []ActionIn{{ID: "review-open", Label: "Review open deals", Count: 2, Cell: "metric:open_pipeline"}}
	return in
}

func assessmentReply(cell, action string) string {
	return `{"paragraphs":[[{"text":"The quarter is moving.","kind":"observation","cells":["` + cell + `"],"records":["d1"]}]],` +
		`"action_ids":["` + action + `"],"next_decision":{"deal_id":"d1","owner_id":"u1","text":"Ask for the budget owner."},"confidence":"high"}`
}

func TestTheAssessmentRequestSendsTheShapeItsParserReads(t *testing.T) {
	in := schemaInput()
	req := Request(in, string(textlang.English))
	if len(req.ResponseSchema) == 0 {
		t.Fatal("the assessment request carries no response schema, so a decoder may write any shape")
	}
	good := assessmentReply("metric:bookings_won", "review-open")
	written, err := Parse(good, in)
	if err != nil || written.Next == nil || len(written.ActionIDs) != 1 {
		t.Fatalf("the parser did not keep the whole well-formed reply: %+v, %v", written, err)
	}
	if err := schema.ValidateJSON(req.ResponseSchema, good); err != nil {
		t.Errorf("the schema refuses a reply the parser keeps: %v", err)
	}
	none := strings.Replace(good, `{"deal_id":"d1","owner_id":"u1","text":"Ask for the budget owner."}`, "null", 1)
	if err := schema.ValidateJSON(req.ResponseSchema, none); err != nil {
		t.Errorf("the schema refuses a null next decision, which the prompt asks for when none is supported: %v", err)
	}
	if err := schema.ValidateJSON(req.ResponseSchema, strings.Replace(good, `"d1","owner_id"`, `"d9","owner_id"`, 1)); err == nil {
		t.Error("the schema admits a next decision on a deal the request never showed")
	}
	if err := schema.ValidateJSON(req.ResponseSchema, strings.Replace(good, `"confidence":"high"`, `"confidence":"certain"`, 1)); err == nil {
		t.Error("the schema admits a confidence outside the vocabulary")
	}
	flat := strings.Replace(strings.Replace(good, `[[{`, `[{`, 1), `}]],`, `}],`, 1)
	if err := schema.ValidateJSON(req.ResponseSchema, flat); err == nil {
		t.Error("the schema admits a paragraph that is not a list of sentences")
	}
}

// A cell or an action the request did not supply refuses the whole reply, so
// the schema closes both to what the request supplied.
func TestTheAssessmentSchemaAndParserAgreeOnCellsAndActions(t *testing.T) {
	in := schemaInput()
	req := Request(in, string(textlang.English))
	replies := map[string]string{}
	for _, cell := range []string{"metric:bookings_won", "metric:open_pipeline", "metric:invented"} {
		replies["cell "+cell] = assessmentReply(cell, "review-open")
	}
	for _, action := range []string{"review-open", "invented-action"} {
		replies["action "+action] = assessmentReply("metric:bookings_won", action)
	}
	for name, reply := range replies {
		_, parseErr := Parse(reply, in)
		schemaErr := schema.ValidateJSON(req.ResponseSchema, reply)
		if (schemaErr == nil) != (parseErr == nil) {
			t.Errorf("%s: the schema admits it = %v (%v), the parser keeps it = %v (%v)",
				name, schemaErr == nil, schemaErr, parseErr == nil, parseErr)
		}
	}
}
