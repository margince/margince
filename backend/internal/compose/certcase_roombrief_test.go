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

type roomBriefStub struct{ answer string }

func (s roomBriefStub) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: s.answer}, nil
}

const roomBriefFixtureJSON = `{"title":"Rollout","state":"live",
	"buyers":[{"label":"tomas","name":"Tomas Petrauskas","signed_in":true}],
	"documents":[{"label":"msa","title":"MSA redline","group":"legal"}],
	"threads":[
		{"label":"resend","comments":[{"speaker":"buyer","author":"Lena Vogt","on":"2026-10-22","body":"Resend the order form?"}]},
		{"label":"kickoff","comments":[{"speaker":"buyer","author":"Lena Vogt","on":"2026-10-21","body":"Who joins the kickoff?"}]},
		{"label":"cap","document_label":"msa","comments":[{"speaker":"buyer","author":"Tomas Petrauskas","on":"2026-10-20","body":"Clause 11.2 caps liability at fees paid."}]}
	]}`

func prepareRoomBrief(t *testing.T, expected string) *roomBriefCase {
	t.Helper()
	prepared, err := roomBriefCases{}.Prepare(json.RawMessage(roomBriefFixtureJSON), json.RawMessage(expected))
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	c, ok := prepared.(*roomBriefCase)
	if !ok {
		t.Fatalf("Prepare returned %T", prepared)
	}
	return c
}

func evaluateRoomBrief(t *testing.T, c *roomBriefCase, reply string) aitasks.Outcome {
	t.Helper()
	trace, err := c.Run(context.Background(), roomBriefStub{answer: reply})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return c.Evaluate(trace)
}

func TestTheRoomBriefCaseGradesEachKindOfReply(t *testing.T) {
	c := prepareRoomBrief(t, `{"cites_label":"cap","names_token":"liability"}`)
	cite := func(id, text string) string {
		return `{"sentences":[{"text":"` + text + `","nature":"fact","evidence":[{"entity_type":"thread","entity_id":"` + id + `"}]}]}`
	}
	capID := c.mustCite
	for name, tc := range map[string]struct {
		reply string
		want  string
	}{
		"reads the blocker":   {cite(capID, "Tomas is waiting on the liability cap."), aitasks.OutcomeAccepted},
		"generic":             {cite(capID, "Tomas is waiting on you."), aitasks.OutcomeWrongAnswer},
		"cites nothing known": {cite("0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e07", "The liability cap."), aitasks.OutcomeAbstained},
		"not JSON":            {"the room is fine", aitasks.OutcomeInvalid},
	} {
		if got := evaluateRoomBrief(t, c, tc.reply); got.Result != tc.want {
			t.Errorf("%s: %s (%s), want %s", name, got.Result, got.Detail, tc.want)
		}
	}
}

func TestTheRoomBriefCaseRefusesScenariosThatMeasureNothing(t *testing.T) {
	for name, expected := range map[string]string{
		"a label the room lacks":   `{"cites_label":"nope","names_token":"liability"}`,
		"a token nobody wrote":     `{"cites_label":"cap","names_token":"indemnity"}`,
		"a token the floor quotes": `{"cites_label":"cap","names_token":"order form"}`,
	} {
		_, err := roomBriefCases{}.Prepare(json.RawMessage(roomBriefFixtureJSON), json.RawMessage(expected))
		if err == nil || !strings.Contains(err.Error(), "deal_room_brief") {
			t.Errorf("%s: Prepare = %v, want a refusal naming the site", name, err)
		}
	}
}
