// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type filterReply struct{ text string }

func (c filterReply) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: c.text}, nil
}

const proposeFixture = `{"resource":"contact","text":"contacts in Germany or Austria","today":"2026-09-30","lang":"en",
 "fields":[{"name":"country","type":"text","operators":["eq","neq","in","contains","exists"]},
           {"name":"lifecycle","type":"picklist","operators":["eq","in"],"options":["prospect","customer"]}]}`

func proposeCase(t *testing.T, expected string) aitasks.PreparedCase {
	t.Helper()
	prepared, err := filterProposeCases{}.Prepare(json.RawMessage(proposeFixture), json.RawMessage(expected))
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	return prepared
}

func evaluateReply(t *testing.T, prepared aitasks.PreparedCase, reply string) aitasks.Outcome {
	t.Helper()
	trace, err := prepared.Run(context.Background(), filterReply{text: reply})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return prepared.Evaluate(trace)
}

func clauseJSON(field, value string) string {
	return `{"phrase":"","field":"` + field + `","op":"eq","text":` + value +
		`,"number":null,"flag":null,"list":null,"days_ago":null}`
}

func replyJSON(groupJoin string, clauses []string, unsupported string) string {
	return `{"groups":[{"join":"` + groupJoin + `","clauses":[` + strings.Join(clauses, ",") +
		`]}],"join":"and","unsupported":` + unsupported + `}`
}

func TestTheFilterCaseReadsATreeForWhatItSelects(t *testing.T) {
	prepared := proposeCase(t, `{"filter":{"and":[{"field":"country","op":"in","value":["DE","AT"]}]},"unsupported":0}`)
	germany, austria := clauseJSON("country", `"de"`), clauseJSON("country", `"AT"`)
	cases := map[string]struct {
		reply string
		want  string
	}{
		"an or of equalities":           {replyJSON("or", []string{germany, austria}, `[]`), aitasks.OutcomeAccepted},
		"an and of the same equalities": {replyJSON("and", []string{germany, austria}, `[]`), aitasks.OutcomeWrongAnswer},
		"the right tree and a phrase it invented to decline": {
			replyJSON("or", []string{germany, austria}, `[{"phrase":"contacts","reason":"vague"}]`), aitasks.OutcomeWrongAnswer,
		},
		"a reply that is not the shape": {`{"clauses":[]}`, aitasks.OutcomeInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := evaluateReply(t, prepared, tc.reply); got.Result != tc.want {
				t.Errorf("reported %q (%s), want %q", got.Result, got.Detail, tc.want)
			}
		})
	}
}

func TestTheFilterCaseScoresAValueTheGateDropsAsNamedBack(t *testing.T) {
	prepared := proposeCase(t, `{"filter":null,"unsupported":1}`)
	reply := replyJSON("and", []string{clauseJSON("lifecycle", `"hot lead"`)}, `[]`)
	if got := evaluateReply(t, prepared, reply); got.Result != aitasks.OutcomeAccepted {
		t.Errorf("a picklist value the gate dropped reported %q (%s), want accepted", got.Result, got.Detail)
	}
	guessed := replyJSON("and", []string{clauseJSON("lifecycle", `"prospect"`)}, `[]`)
	if got := evaluateReply(t, prepared, guessed); got.Result != aitasks.OutcomeWrongAnswer {
		t.Errorf("the nearest option guessed in reported %q, want wrong_answer", got.Result)
	}
}

func TestTheFilterCaseRefusesAnExpectationTheVocabularyCannotReach(t *testing.T) {
	for name, expected := range map[string]string{
		"an unknown field":      `{"filter":{"and":[{"field":"purchase_intent","op":"eq","value":"high"}]},"unsupported":0}`,
		"an operator not given": `{"filter":{"and":[{"field":"lifecycle","op":"neq","value":"customer"}]},"unsupported":0}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := (filterProposeCases{}).Prepare(json.RawMessage(proposeFixture), json.RawMessage(expected)); err == nil {
				t.Error("Prepare accepted an expectation no model answer could satisfy")
			}
		})
	}
}
