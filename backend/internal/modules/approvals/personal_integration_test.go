// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestPersonalDecisionsRemainReachableBehindATeamBacklog(t *testing.T) {
	e := setupStaging(t)
	target := newDealSeeder(t, e).deal(t)
	agent := e.asAgent(t)
	personal := map[ids.UUID]bool{}
	stage := func(kind string, sequence int) ids.UUID {
		t.Helper()
		id, err := e.svc.Stage(agent, StageInput{
			Kind: kind, ProposedChange: []byte(`{"body":"Review the proposal."}`),
			DiffHash: fmt.Sprintf("personal-queue-%d", sequence), TargetType: tableDeal, TargetID: target,
			Summary: "Review the proposal",
		})
		if err != nil {
			t.Fatal(err)
		}
		return id.UUID
	}
	for i := range 2 {
		personal[stage(kindHeldDraft, i)] = true
	}
	var team ids.UUID
	for i := range inboxBatch + 1 {
		team = stage("advance_deal", i+2)
	}
	reader := seatFor(e.ws, e.rep, tableDeal, objectActivity)
	all, _, err := e.svc.ListWire(reader, ListInput{Limit: 1})
	if err != nil || len(all) != 1 || ids.UUID(all[0].Id) != team || all[0].RequestedDeciderId != nil {
		t.Fatalf("team proposal is not reachable in the full inbox: %v", err)
	}
	for _, targeted := range []bool{false, true} {
		t.Run(fmt.Sprintf("targeted_%t", targeted), func(t *testing.T) {
			in := ListInput{RequestedForReader: true, Limit: 1}
			if targeted {
				in.TargetType, in.TargetID = new(tableDeal), &target
			}
			found := map[ids.UUID]bool{}
			for pageNumber := range 5 {
				rows, page, err := e.svc.ListWire(reader, in)
				if err != nil {
					t.Fatal(err)
				}
				if !targeted && pageNumber == 0 && len(rows) != 1 {
					t.Fatal("the team backlog starved the personal inbox")
				}
				for _, row := range rows {
					id := ids.UUID(row.Id)
					if !personal[id] || found[id] {
						t.Fatalf("personal queue returned an unrelated or repeated decision: %s", id)
					}
					if row.RequestedDeciderId == nil || ids.UUID(*row.RequestedDeciderId) != e.rep {
						t.Fatal("personal wire row lost its requested decider")
					}
					found[id] = true
				}
				if !page.HasMore {
					if len(found) != len(personal) {
						t.Fatalf("found %d personal decisions, want %d", len(found), len(personal))
					}
					return
				}
				if page.NextCursor == "" || page.NextCursor == in.Cursor {
					t.Fatal("personal pagination did not advance")
				}
				in.Cursor = page.NextCursor
			}
			t.Fatal("personal pagination did not finish")
		})
	}
}
