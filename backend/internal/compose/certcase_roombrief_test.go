// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/roombrief"
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

// The opening check reads the first sentence, which is where a reader looks.
//
// Asked of the whole brief, both halves pass on a brief that opens with the
// newest message and mentions the condition later. They also pass on one
// crediting the other buyer in the same thread.
func TestTheRoomBriefOpeningIsReadForBothTheConditionAndItsAuthor(t *testing.T) {
	c := &roomBriefCase{mustLead: "penetration", mustAttribute: "Rasmus"}
	for _, tc := range []struct {
		name    string
		opening string
		faults  int
	}{
		{"the condition and the buyer who set it", "rasmus dahl set a penetration test condition", 0},
		{"the other buyer in that thread", "aino lehtinen set a penetration test condition", 1},
		{"the newest ask instead", "rasmus dahl asked for the migration schedule", 1},
		{"neither", "the room is quiet", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if faults := c.openingFaults(tc.opening); len(faults) != tc.faults {
				t.Errorf("%q reported %d faults (%v), want %d", tc.opening, len(faults), faults, tc.faults)
			}
		})
	}
}

// An expectation the fixture cannot answer is refused, so a case that can
// never pass is rejected rather than scored as a model failure.
func TestARoomBriefOpeningExpectationMustBeAnswerable(t *testing.T) {
	err := refuseUnmeasurableOpening(`{"threads":[{"body":"a penetration test"}]}`,
		roombrief.Input{}, roomBriefExpectation{LeadsWith: "indemnity"})
	if err == nil {
		t.Error("an expectation leading with a word the summary lacks was accepted")
	}
	if err := refuseUnmeasurableOpening(`{"threads":[{"body":"a penetration test"}]}`,
		roombrief.Input{}, roomBriefExpectation{LeadsWith: "penetration"}); err != nil {
		t.Errorf("an answerable expectation was refused: %v", err)
	}
}
