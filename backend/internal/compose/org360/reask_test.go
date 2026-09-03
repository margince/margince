// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// The buying-role read asks through ai.Ask, so a reply proposeroles.Parse
// refuses goes back to the model rather than leaving the account unread.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/proposeroles"
	"github.com/margince/margince/backend/internal/modules/ai/aitest"
)

func TestTheBuyingRoleReadReAsksThroughItsOwnParse(t *testing.T) {
	t.Parallel()
	lane := &aitest.ReAsking{
		First: "Sofia looks like the economic buyer here.",
		Second: `{"proposals":[{"person_id":"p-1","role":"economic_buyer",` +
			`"evidence_snippet":"I sign off on this budget.","source_id":"m-1","confidence":0.8}]}`,
	}
	proposals, err := readProposals(t.Context(), lane, "Nordwind", []proposeroles.Candidate{
		{PersonID: "p-1", FullName: "Sofia Brandt", Title: "CFO"},
	})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if lane.Refused == nil {
		t.Fatal("the site accepted a reply proposeroles.Parse refuses, so its validator is looser than its own read")
	}
	if lane.Bare != 0 {
		t.Errorf("the bare lane was taken %d time(s) on a lane that can re-ask", lane.Bare)
	}
	if len(proposals) != 1 || proposals[0].Role != "economic_buyer" {
		t.Fatalf("the re-asked proposals did not come back: %+v", proposals)
	}
}
