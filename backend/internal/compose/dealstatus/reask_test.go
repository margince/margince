// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// The status writer asks through ai.Ask, so a reply ParseStatus refuses goes
// back to the model rather than dropping the card to its deterministic floor.
//
// Only a lane that can re-ask reaches the validator at all, so without this the
// site could hand the lane a looser check than its own parse and every other
// test here would still pass.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai/aitest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestTheStatusWriterReAsksThroughItsOwnParse(t *testing.T) {
	t.Parallel()
	in := inputWithTimeline()
	lane := &aitest.ReAsking{
		// Valid JSON, refused by ParseStatus alone: story is a string where the
		// site reads a list of evidenced lines.
		First: `{"story":"The offer was never sent."}`,
		Second: reply(draft{
			story:      []map[string]any{line("The offer was promised on the call and never sent.", "act-1")},
			blocker:    []map[string]any{line("Nothing has moved in twelve days.", "act-2")},
			buyer:      []map[string]any{line("They asked for the price before anything else.", "act-2")},
			standing:   "drifting",
			because:    []map[string]any{line("The last contact was the call, and nobody followed it.", "act-1")},
			moveReason: []map[string]any{line("Sending it is what the call promised.", "act-1")},
		}),
	}
	service := &Service{lane: lane}

	written, err := service.ask(t.Context(), in, string(textlang.English))
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if lane.Refused == nil {
		t.Fatal("the site accepted a reply ParseStatus refuses, so its validator is looser than its own read")
	}
	if lane.Bare != 0 {
		t.Errorf("the bare lane was taken %d time(s) on a lane that can re-ask", lane.Bare)
	}
	if len(written.Story) == 0 {
		t.Fatal("the re-asked status carried no story, so the second reply never landed")
	}
}
