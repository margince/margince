// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgbrief

// Both model paths here ask through ai.Ask, so a reply their own parse refuses
// goes back to the model rather than dropping the reader to the floor.
//
// The assertion is on ReAsking.Refused. An ordinary fake never calls the
// validator at all, so a site that handed the lane a looser check than its own
// parse would pass every other test in this package unchanged.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai/aitest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const groundedBriefReply = `{"sections":[
	{"kind":"snapshot","sentences":[
		{"text":"They sell managed hosting.","evidence":[{"entity_type":"organization","entity_id":"` +
	briefOrgID + `"}]}]}]}`

func TestTheBriefWriterReAsksThroughItsOwnParse(t *testing.T) {
	t.Parallel()
	lane := &aitest.ReAsking{
		// Valid JSON, refused by ParseBriefSections alone: sections is an object
		// where the site reads a list.
		First:  `{"sections":{"kind":"snapshot"}}`,
		Second: groundedBriefReply,
	}
	sections, err := writeWithModel(t.Context(), lane, briefOrgID, inputFixture(), string(textlang.English))
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
	if lane.Refused == nil {
		t.Fatal("the site accepted a reply ParseBriefSections refuses, so its validator is looser than its own read")
	}
	if lane.Bare != 0 {
		t.Errorf("the bare lane was taken %d time(s) on a lane that can re-ask", lane.Bare)
	}
	if len(sections) == 0 {
		t.Fatal("the re-asked brief carried no section, so the second reply never landed")
	}
}

func TestTheQuestionAnswererReAsksThroughItsOwnParse(t *testing.T) {
	t.Parallel()
	lane := &aitest.ReAsking{
		// Valid JSON, refused by ParseBrief alone: sentences is a string where
		// the site reads a list.
		First: `{"sentences":"They sell managed hosting."}`,
		Second: `{"sentences":[{"text":"They sell managed hosting.","evidence":[` +
			`{"entity_type":"organization","entity_id":"` + askOrgID + `"}]}]}`,
	}
	answered, err := answerWithModel(t.Context(), lane, declaredQuestions(t)[0], askOrgID,
		askInput(), string(textlang.English))
	if err != nil {
		t.Fatalf("answering: %v", err)
	}
	if lane.Refused == nil {
		t.Fatal("the site accepted a reply ParseBrief refuses, so its validator is looser than its own read")
	}
	if len(answered) == 0 {
		t.Fatal("the re-asked answer carried no sentence, so the second reply never landed")
	}
}
