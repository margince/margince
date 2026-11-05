// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCurrentMovesFollowSourceChangesWithoutWaitingForEvents(t *testing.T) {
	for _, change := range []string{"complete", "archive", "relink", "edit"} {
		t.Run(change, func(t *testing.T) {
			e := integration.Setup(t)
			pipeline, stage, _ := integration.DealFixture(t, e)
			deal := e.SeedDeal(t, "Demo expansion", pipeline, stage, &e.Rep1)
			other := e.SeedDeal(t, "Other expansion", pipeline, stage, &e.Rep1)
			at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
			store := activities.NewStore(e.DB()).WithClock(func() time.Time { return at })
			subject := "Agree the next delivery"
			task, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: "task", Subject: &subject, Source: "manual",
				Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
			})
			if err != nil {
				t.Fatal(err)
			}
			reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
			cards := newDealStatusService(e.Pool, func() time.Time { return at })
			before, err := cards.ReadFacts(reader, ids.From[ids.DealKind](deal))
			if err != nil || before.Next == nil || before.Next.Action != dealstatus.ActionOpenTask {
				t.Fatalf("warming actual task card: %+v, %v", before.Next, err)
			}
			id := ids.From[ids.ActivityKind](ids.UUID(task.Id))
			switch change {
			case "complete":
				done := true
				_, err = store.UpdateActivity(e.Admin(), id, activities.UpdateActivityInput{IsDone: &done})
			case "archive":
				_, err = store.ArchiveActivity(e.Admin(), id, nil)
			case "relink":
				_, err = store.RelinkActivity(e.Admin(), id, activities.RelinkActivityInput{EntityType: "deal", EntityID: other, ReplaceExistingOfType: true})
			case "edit":
				subject = "Confirm the revised delivery"
				_, err = store.UpdateActivity(e.Admin(), id, activities.UpdateActivityInput{Subject: &subject})
			}
			if err != nil {
				t.Fatal(err)
			}
			moves, err := cards.CurrentMoves(reader, []ids.UUID{deal, other})
			if err != nil {
				t.Fatal(err)
			}
			current := moves[deal]
			if current.Recommendation != nil && current.Recommendation.BasisFingerprint == before.Next.Recommendation.BasisFingerprint {
				t.Fatal("source changed while its recommendation basis stayed the same")
			}
			named, present := dealstatus.NamedActivity(current)
			if change == "edit" {
				if !present || named != id.UUID || len(current.Evidence) == 0 || current.Evidence[0].Text != subject {
					t.Fatalf("edited task was not re-read: %+v", current)
				}
			} else if present && named == id.UUID {
				t.Fatalf("obsolete source still supplies the deal operand: %+v", current)
			}
			if change == "relink" {
				if named, present := dealstatus.NamedActivity(moves[other]); !present || named != id.UUID {
					t.Fatal("a current relink failed to move the advice to its actual deal")
				}
			}
		})
	}
}
