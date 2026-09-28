// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// askOnlyLane can answer an LLM request and nothing else: the shape of the
// offline fake, the certification recorder and every test double.
type askOnlyLane struct {
	asked []model.Request
	reply model.Response
}

func (l *askOnlyLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.asked = append(l.asked, req)
	return l.reply, nil
}

// A lane that cannot decide is asked the site's LLM question exactly as Ask
// would ask it, and the outcome says no decision was made — so a site reads
// its answer from the ladder's reply, never from an empty Answer.
func TestDecideAsksTheLadderWhenTheLaneCannotDecide(t *testing.T) {
	lane := &askOnlyLane{reply: model.Response{Text: `{"kind":"company"}`, ServedModel: "served-by-ladder"}}
	req := model.Request{System: "classify the site", Messages: []model.Message{{Role: "user", Content: "page"}}}
	dreq := decision.Request{
		Model: "jev",
		State: json.RawMessage(`{"page":"page"}`),
		Questions: map[string]decision.Question{
			"kind": {Type: decision.Choice, Instructions: "what the site is", Criteria: map[string]string{"company": "a company"}},
		},
	}
	gateCalled := false
	gate := func(decision.Answer) DecisionVerdict {
		gateCalled = true
		return DecisionAccepted
	}

	out, err := Decide(context.Background(), lane, "triage", dreq, req, nil, gate)
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if out.Decided {
		t.Error("a lane with no decision form reported a decision")
	}
	if gateCalled {
		t.Error("the decision gate ran although no decision was asked")
	}
	if len(lane.asked) != 1 || !reflect.DeepEqual(lane.asked[0], req) {
		t.Errorf("the ladder was asked %+v, want exactly the site's LLM request %+v", lane.asked, req)
	}
	if out.Response.Text != lane.reply.Text || out.ServedModel != "served-by-ladder" {
		t.Errorf("outcome = %+v, want the ladder's reply and served model", out)
	}
}
