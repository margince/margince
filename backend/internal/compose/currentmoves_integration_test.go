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

func TestCurrentMovesFollowTheOwnerWithoutAWrittenOrReadableCache(t *testing.T) {
	for _, cache := range []string{"absent", "obsolete", "unreadable"} {
		t.Run(cache, func(t *testing.T) {
			e := setupVerdicts(t)
			reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
			at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
			cards := newDealStatusService(e.Pool, func() time.Time { return at })
			if cache != "absent" {
				card, err := cards.ReadFacts(reader, ids.From[ids.DealKind](e.calm))
				if err != nil || card.Next == nil || card.Next.Action != dealstatus.ActionCreateTask {
					t.Fatalf("warming the real card: %+v, %v", card.Next, err)
				}
			}
			if cache == "unreadable" {
				e.WsExec(t, `UPDATE deal_status_card SET payload='{"card":true}'::jsonb WHERE deal_id=$1`, e.calm)
			}
			subject := "Send the agreed scope"
			task, _, err := activities.NewStore(e.DB()).WithClock(func() time.Time { return at }).LogActivity(reader, activities.LogActivityInput{
				Kind: "task", Source: "manual", Subject: &subject,
				Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: e.calm}},
			})
			if err != nil {
				t.Fatal(err)
			}
			moves, err := cards.CurrentMoves(reader, []ids.UUID{e.calm})
			if err != nil {
				t.Fatal(err)
			}
			move, found := moves[e.calm]
			named, present := dealstatus.NamedActivity(move)
			if !found || move.Action != dealstatus.ActionOpenTask || !present || named != ids.UUID(task.Id) || move.Recommendation == nil {
				t.Fatalf("current owner work was replaced by cached advice: %+v", moves)
			}
			if cache == "absent" && e.WsCount(t, "SELECT count(*) FROM deal_status_card WHERE deal_id=$1", e.calm) != 0 {
				t.Fatal("current action selection wrote an unnecessary model card")
			}
		})
	}
}
