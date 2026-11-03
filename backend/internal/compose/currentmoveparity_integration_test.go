// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheRecordAndBatchShareTheSameRecommendationBasis(t *testing.T) {
	for _, scenario := range []string{"empty", "champion", "outreach", "task", "meeting", "far_meeting", "meeting_followup"} {
		t.Run(scenario, func(t *testing.T) {
			e := integration.Setup(t)
			pipeline, stage, _ := integration.DealFixture(t, e)
			deal := ids.From[ids.DealKind](e.SeedDeal(t, "Demo expansion", pipeline, stage, &e.Rep1))
			at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
			store := activities.NewStore(e.DB()).WithClock(func() time.Time { return at })
			reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
			switch scenario {
			case "champion", "outreach", "meeting_followup":
				contact := ids.From[ids.ContactKind](e.SeedContact(t, "Demo champion", &e.Rep1))
				role := "champion"
				if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
					Kind: "deal_stakeholder", ContactID: &contact, DealID: &deal, Role: &role, Source: "manual",
				}); err != nil {
					t.Fatal(err)
				}
				if scenario == "meeting_followup" {
					meetingAt, status, duration, subject := at.Add(-72*time.Hour), "held", 3600, "Scope review"
					host := ids.From[ids.UserKind](e.Rep1)
					if _, _, err := store.LogActivity(reader, activities.LogActivityInput{Kind: "meeting", Subject: &subject, OccurredAt: &meetingAt, MeetingStatus: &status, DurationSeconds: &duration, HostUserID: &host, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal.UUID}, {EntityType: "contact", EntityID: contact.UUID}}}); err != nil {
						t.Fatal(err)
					}
				}
				calledAt := at.Add(-48 * time.Hour)
				subject := "Scope discussion"
				if scenario != "outreach" {
					if _, _, err := store.LogActivity(reader, activities.LogActivityInput{
						Kind: "call", Subject: &subject, OccurredAt: &calledAt, Source: "manual",
						Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal.UUID}},
					}); err != nil {
						t.Fatal(err)
					}
				}
			}
			if scenario == "task" || scenario == "meeting" || scenario == "far_meeting" {
				subject, status, starts := "Agreed delivery", "booked", at.Add(24*time.Hour)
				kind := scenario
				if scenario == "far_meeting" {
					kind = "meeting"
					starts = at.Add(90 * 24 * time.Hour)
				}
				in := activities.LogActivityInput{
					Kind: kind, Subject: &subject, Source: "manual",
					Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal.UUID}},
				}
				if kind == "meeting" {
					in.OccurredAt, in.MeetingStatus = &starts, &status
				}
				if _, _, err := store.LogActivity(reader, in); err != nil {
					t.Fatal(err)
				}
			}
			cards := newDealStatusService(e.Pool, func() time.Time { return at })
			card, err := cards.ReadFacts(reader, deal)
			if err != nil || card.Next == nil || card.Next.Recommendation == nil {
				t.Fatalf("record action: %+v, %v", card.Next, err)
			}
			moves, err := cards.CurrentMoves(reader, []ids.UUID{deal.UUID})
			if err != nil {
				t.Fatal(err)
			}
			batch := moves[deal.UUID]
			if !reflect.DeepEqual(card.Next.Recommendation, batch.Recommendation) || card.Next.Action != batch.Action {
				t.Fatalf("record and batch disagree: record=%+v, batch=%+v", card.Next, batch)
			}
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/status", nil).WithContext(reader)
			receipt := batch.Recommendation
			dealstatus.NewHandlers(cards).GetDealStatus(response, request, crmcontracts.Id(deal.UUID), crmcontracts.GetDealStatusParams{FactsOnly: new(true), IfRule: &receipt.Rule, IfRecommendation: &receipt.BasisFingerprint})
			if response.Code != http.StatusOK {
				t.Fatalf("the record endpoint refused its batch receipt: %d %s", response.Code, response.Body.String())
			}
			if scenario == "champion" && card.Next.Action == dealstatus.ActionCreateTask {
				store.WithTaskRecommendation(taskRecommendationFrom(cards))
				if _, created, err := store.LogActivity(reader, suggestedTaskInput(t, card.Next)); err != nil || !created {
					t.Fatalf("the common writer refused an unchanged named suggestion: created=%t, %v", created, err)
				}
			}
		})
	}
}
