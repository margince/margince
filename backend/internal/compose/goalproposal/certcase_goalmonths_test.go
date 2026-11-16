// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package goalproposal

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const autumnFixture = `{"year":2027,"currency":"EUR","today":"2026-11-12","closed_revenue":[
  {"year":2027,"months":[null,null,null,null,null,null,null,null,null,null,null,null]},
  {"year":2026,"months":[10000,10000,12000,12000,8000,6000,6000,8000,20000,24000,null,null]}]}`

const autumnExpectation = `{"above":[9,10],"below":[6,7],"total_min":120000,"total_max":200000}`

type fixedReply string

func (r fixedReply) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: string(r)}, nil
}

func judge(t *testing.T, reply string) aitasks.Outcome {
	t.Helper()
	prepared, err := Cases{}.Prepare(json.RawMessage(autumnFixture), json.RawMessage(autumnExpectation))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	trace, err := prepared.Run(context.Background(), fixedReply(reply))
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Requests) != 1 || !strings.HasPrefix(trace.Requests[0].System, system) || !strings.Contains(trace.Requests[0].Messages[0].Content, `"year":2026`) {
		t.Fatal("the case does not send the request production sends")
	}
	return prepared.Evaluate(trace)
}

func TestTheCaseAcceptsAPlanThatFollowsThePattern(t *testing.T) {
	outcome := judge(t, `{"months":[10000,10000,12000,12000,9000,7000,7000,9000,20000,24000,15000,15000],"reason":"Autumn carries the year after a quiet summer."}`)
	if outcome.Result != aitasks.OutcomeAccepted {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestTheCaseTellsAWrongPlanFromAnInvalidOne(t *testing.T) {
	flat := judge(t, `{"months":[12500,12500,12500,12500,12500,12500,12500,12500,12500,12500,12500,12500],"reason":"The months are even."}`)
	if flat.Result != aitasks.OutcomeWrongAnswer {
		t.Errorf("an even plan over a seasonal year = %+v, want a wrong answer", flat)
	}
	inflated := judge(t, `{"months":[10000,10000,12000,12000,9000,7000,7000,9000,200000,240000,15000,15000],"reason":"Autumn carries the year."}`)
	if inflated.Result != aitasks.OutcomeWrongAnswer {
		t.Errorf("a plan at four times the closed level = %+v, want a wrong answer", inflated)
	}
	if invalid := judge(t, `{"months":[1,2,3],"reason":"Short."}`); invalid.Result != aitasks.OutcomeInvalid {
		t.Errorf("three months = %+v, want invalid", invalid)
	}
}

func TestPrepareRefusesAScenarioThatMeasuresNothing(t *testing.T) {
	for name, scenario := range map[string][2]string{
		"no closed revenue":  {`{"year":2027,"currency":"EUR","closed_revenue":[{"year":2026,"months":[0,0]}]}`, autumnExpectation},
		"nothing expected":   {autumnFixture, `{}`},
		"a thirteenth month": {autumnFixture, `{"above":[13],"below":[1]}`},
		"one side only":      {autumnFixture, `{"above":[9]}`},
	} {
		if _, err := (Cases{}).Prepare(json.RawMessage(scenario[0]), json.RawMessage(scenario[1])); err == nil {
			t.Errorf("%s was prepared", name)
		}
	}
}
