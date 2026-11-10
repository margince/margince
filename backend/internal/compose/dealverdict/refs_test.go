// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import (
	"slices"
	"strings"
	"testing"
)

const (
	activityID  = "0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e07"
	stageID     = "0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e08"
	criterionID = "0198f3a1-7c42-7e0b-9d51-2a6f4b8c1e09"
)

func refInput() Input {
	in := guardInput()
	in.Activities[0].ID = activityID
	in.Later = []StageOption{{ID: stageID, Name: "Negotiation", Position: 2, Probability: 75}}
	in.ExitCriteria = []ExitCriterion{{ID: criterionID, StagePosition: 1, Stage: "Proposal", Label: "Budget", Kind: "custom"}}
	return in
}

// The model sees the deal's records by ref only, so it has no id to garble
// or copy into its reason.
func TestTheRequestShowsRefsAndNoRecordID(t *testing.T) {
	t.Parallel()
	req := Request(refInput(), "en")
	for _, id := range []string{activityID, stageID, criterionID} {
		if strings.Contains(req.Messages[0].Content, id) || strings.Contains(string(req.ResponseSchema), id) {
			t.Errorf("the request shows the model the record id %s", id)
		}
	}
	for _, shown := range []string{`activity_id="a1"`, "stage_id s1", "criterion_id c1"} {
		if !strings.Contains(req.Messages[0].Content, shown) {
			t.Errorf("the prompt does not show %q:\n%s", shown, req.Messages[0].Content)
		}
	}
}

func TestAMoveNamedByRefReadsBackToTheRecordIDs(t *testing.T) {
	t.Parallel()
	text := `{"actions":[{"kind":"move_stage","date":"","amount":0,"currency":"","quoted_words":"","probability":0,` +
		`"stage_id":"s1","criteria":[{"criterion_id":"c1","activity_id":"a1","quoted_words":"sign"}],` +
		`"cited_activity_ids":["a1"],"reason":"r","confidence":0.9}]}`
	payload, err := Parse(text, refInput())
	if err != nil {
		t.Fatal(err)
	}
	move := payload.Actions[0]
	if move.StageID != stageID || !slices.Equal(move.Cited, []string{activityID}) ||
		move.Criteria[0].CriterionID != criterionID || move.Criteria[0].ActivityID != activityID {
		t.Errorf("the refs read back as %+v, want the record ids", move)
	}
}

func TestARefTheRequestNeverShowedIsRefused(t *testing.T) {
	t.Parallel()
	for name, fields := range map[string]string{
		"an activity id in place of its ref": `"stage_id":"","criteria":[],"cited_activity_ids":["` + activityID + `"]`,
		"an activity ref not shown":          `"stage_id":"","criteria":[],"cited_activity_ids":["a2"]`,
		"a stage ref not shown":              `"stage_id":"s2","criteria":[],"cited_activity_ids":["a1"]`,
		"a criterion ref not shown": `"stage_id":"s1","criteria":[{"criterion_id":"c2","activity_id":"a1",` +
			`"quoted_words":"sign"}],"cited_activity_ids":["a1"]`,
	} {
		text := `{"actions":[{"kind":"set_probability","date":"","amount":0,"currency":"","quoted_words":"",` +
			`"probability":30,` + fields + `,"reason":"r","confidence":0.9}]}`
		if _, err := Parse(text, refInput()); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
