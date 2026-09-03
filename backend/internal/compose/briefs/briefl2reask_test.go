// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package briefs

// The L2 re-order asks through ai.Ask, so a reply it cannot read goes back to
// the model instead of dropping the whole pass to the deterministic order.

import (
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai/aitest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheReOrderReAsksThroughItsOwnParse(t *testing.T) {
	t.Parallel()
	first, second := ids.NewV7(), ids.NewV7()
	lane := &aitest.ReAsking{
		First:  "here you go: the best ones first",
		Second: `{"order":["` + second.String() + `","` + first.String() + `"]}`,
	}
	ranker := briefL2Ranker{brain: lane, log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	order, err := ranker.askModel(t.Context(), []BriefQueueItem{
		{DealID: first}, {DealID: second},
	})
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if lane.Refused == nil {
		t.Fatal("the site accepted a reply ParseRankOrder refuses, so the validator it hands the lane is " +
			"looser than its own read and a malformed reply would never be re-asked")
	}
	if lane.Bare != 0 {
		t.Errorf("the bare lane was taken %d time(s) on a lane that can re-ask", lane.Bare)
	}
	if len(order) != 2 || order[0] != second {
		t.Fatalf("the re-asked order did not come back: %v", order)
	}
}
