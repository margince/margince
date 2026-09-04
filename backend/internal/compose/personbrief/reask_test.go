// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personbrief

// The model path asks through ai.Ask, so a reply its own parse refuses goes
// back to the model rather than dropping the reader to the floor.
//
// The assertion is on ReAsking.Refused. An ordinary fake never calls the
// validator at all, so a site that handed the lane a looser check than its own
// parse would pass every other test in this package unchanged.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai/aitest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestTheBriefWriterReAsksThroughItsOwnParse(t *testing.T) {
	t.Parallel()
	lane := &aitest.ReAsking{
		// Valid JSON, refused by ParseBrief alone: sentences is a string where
		// the site reads a list.
		First: `{"sentences":"They are waiting on the list."}`,
		Second: `{"sentences":[{"text":"They are waiting on the list.","evidence":[` +
			`{"entity_type":"activity","entity_id":"` + objectionID + `"}]}]}`,
	}
	written, err := writeWithModel(t.Context(), lane, briefPersonID, inputFixture(),
		string(textlang.English))
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
	if lane.Refused == nil {
		t.Fatal("the site accepted a reply ParseBrief refuses, so its validator is looser than its own read")
	}
	if lane.Bare != 0 {
		t.Errorf("the bare lane was taken %d time(s) on a lane that can re-ask", lane.Bare)
	}
	if len(written) == 0 {
		t.Fatal("the re-asked brief carried no sentence, so the second reply never landed")
	}
}
