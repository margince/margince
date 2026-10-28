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

func TestTheRecordAndBatchChooseTheReadableRequestBeforeTheirLimit(t *testing.T) {
	for _, mode := range []string{"current", "review", "reminder"} {
		t.Run(mode, func(t *testing.T) {
			review := mode == "review"
			e := integration.Setup(t)
			pipeline, stage, _ := integration.DealFixture(t, e)
			deal := e.SeedDeal(t, "Demo renewal", pipeline, stage, &e.Rep1)
			at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
			reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
			owner := integration.OwnerConn(t)
			var recipient string
			if err := owner.QueryRow(t.Context(), "SELECT email FROM app_user WHERE id=$1", e.Rep1).Scan(&recipient); err != nil {
				t.Fatal(err)
			}
			store := activities.NewStore(e.DB()).WithClock(func() time.Time { return at })
			var visible ids.UUID
			for index := range 3 {
				occurred := at.Add(time.Duration(index-3) * 24 * time.Hour)
				if review {
					occurred = occurred.Add(-180 * 24 * time.Hour)
				}
				direction, subject := "inbound", "Please send the pricing annex"
				row, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
					Kind: "email", Direction: &direction, Subject: &subject, OccurredAt: &occurred, Source: "manual",
					EmailFrom: "buyer@example.test", EmailTo: []string{recipient}, ThreadKey: ids.NewV7().String(),
					Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
				})
				if err != nil {
					t.Fatal(err)
				}
				id := ids.UUID(row.Id)
				if _, err := store.SetOwedVerdict(e.Admin(), id, activities.OwedVerdictAsksUs, "prompts-test", at); err != nil {
					t.Fatal(err)
				}
				if index == 1 {
					visible = id
					continue
				}
				if _, err := store.SetAudience(e.Admin(), ids.From[ids.ActivityKind](id), activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.AdminUser}}}); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "reminder" {
				other := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
				task, _, err := store.LogActivity(other, activities.LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &visible})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := store.SetAudience(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(task.Id)), activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep2}}}); err != nil {
					t.Fatal(err)
				}
			}
			cards := newDealStatusService(e.Pool, func() time.Time { return at })
			record, err := cards.ReadFacts(reader, ids.From[ids.DealKind](deal))
			if err != nil || record.Next == nil {
				t.Fatalf("record suggestion: %+v %v", record.Next, err)
			}
			named, ok := dealstatus.NamedActivity(*record.Next)
			if !ok || named != visible {
				t.Fatalf("record chose no readable request: %+v", record.Next)
			}
			batch, err := cards.CurrentMoves(reader, []ids.UUID{deal})
			if err != nil || batch[deal].Recommendation == nil || record.Next.Recommendation.BasisFingerprint != batch[deal].Recommendation.BasisFingerprint || record.Next.Recommendation.Rule != batch[deal].Recommendation.Rule {
				t.Fatalf("record and batch selected different requests: record=%+v batch=%+v err=%v", record.Next, batch, err)
			}
			if mode == "current" {
				store.WithTaskRecommendation(taskRecommendationFrom(cards))
				if _, created, err := store.LogActivity(reader, suggestedTaskInput(t, record.Next)); err != nil || !created {
					t.Fatalf("current readable request could not be accepted: %t %v", created, err)
				}
			}
		})
	}
}
