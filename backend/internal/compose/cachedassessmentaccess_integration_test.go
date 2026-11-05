// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestAnEarlierAssessmentRequiresItsUncitedInputToRemainReadable(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Demo renewal", pipeline, stage, &e.Rep1)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	store := activities.NewStore(e.DB()).WithClock(func() time.Time { return at })
	var sources []ids.UUID
	for _, subject := range []string{"Scope discussion", "Budget requires approval"} {
		occurred := at.Add(-time.Hour)
		row, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "call", Subject: &subject, Source: "manual", OccurredAt: &occurred,
			Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
		})
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, ids.UUID(row.Id))
	}
	sawUncited := false
	lane := brainFunc(func(_ context.Context, request model.Request) (model.Response, error) {
		for _, message := range request.Messages {
			sawUncited = sawUncited || strings.Contains(message.Content, "Budget requires approval")
		}
		return model.Response{Text: fmt.Sprintf(`{"story":[{"text":"The scope was discussed.","evidence":[%q]}],"blocker":[],"buyer":[],"verdict":{"standing":"drifting","because":[{"text":"The budget requires approval.","evidence":[%q]}]},"move_reason":[]}`, sources[0].String(), sources[0].String())}, nil
	})
	service := newDealStatusService(e.Pool, func() time.Time { return at }).WithLane(lane, "test")
	card, err := service.Get(reader, ids.From[ids.DealKind](deal), true)
	if err != nil {
		t.Fatal(err)
	}
	if !sawUncited || card.GeneratedBy != crmcontracts.WrittenByModel || card.Verdict == nil {
		t.Fatalf("the actual model boundary was not exercised: input=%t card=%+v", sawUncited, card)
	}
	before, err := service.CachedCards(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := before[deal]; !ok {
		t.Fatal("the readable assessment was never served")
	}
	if _, err := store.SetAudience(e.Admin(), ids.From[ids.ActivityKind](sources[1]), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.AdminUser}},
	}); err != nil {
		t.Fatal(err)
	}
	after, err := service.CachedCards(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := after[deal]; ok {
		t.Fatal("an assessment survived loss of its uncited input")
	}
}

func TestAnEarlierAssessmentRequiresItsStakeholderContextToRemainReadable(t *testing.T) {
	for _, loss := range []string{"relationship_grant", "archived_edge"} {
		t.Run(loss, func(t *testing.T) {
			e := integration.Setup(t)
			pipeline, stage, _ := integration.DealFixture(t, e)
			deal := ids.From[ids.DealKind](e.SeedDeal(t, "Demo renewal", pipeline, stage, &e.Rep1))
			contact := ids.From[ids.ContactKind](e.SeedContact(t, "Demo champion", &e.Rep1))
			role := "champion"
			edge, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
				Kind: "deal_stakeholder", ContactID: &contact, DealID: &deal, Role: &role, Source: "manual",
			})
			if err != nil {
				t.Fatal(err)
			}
			at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
			subject, occurred := "Scope discussion", at.Add(-time.Hour)
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: "call", Subject: &subject, Source: "manual", OccurredAt: &occurred,
				Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal.UUID}},
			})
			if err != nil {
				t.Fatal(err)
			}
			lane := brainFunc(func(context.Context, model.Request) (model.Response, error) {
				return model.Response{Text: fmt.Sprintf(`{"story":[{"text":"The scope was discussed.","evidence":[%q]}],"blocker":[],"buyer":[],"verdict":{"standing":"live","because":[{"text":"The scope was discussed with the champion.","evidence":[%q]}]},"move_reason":[]}`, source.Id.String(), source.Id.String())}, nil
			})
			service := newDealStatusService(e.Pool, func() time.Time { return at }).WithLane(lane, "test")
			reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
			card, err := service.Get(reader, deal, true)
			if err != nil || card.GeneratedBy != crmcontracts.WrittenByModel || card.Verdict == nil {
				t.Fatalf("actual model card missing: %+v, %v", card, err)
			}
			before, err := service.CachedCards(reader, []ids.UUID{deal.UUID})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := before[deal.UUID]; !ok {
				t.Fatal("readable stakeholder context was never served")
			}
			if loss == "relationship_grant" {
				permissions := integration.AdminPerms
				permissions.Objects = maps.Clone(permissions.Objects)
				delete(permissions.Objects, "relationship")
				reader = e.As(e.Rep1, []ids.UUID{e.Team1}, permissions)
			} else if _, err := e.Contacts.ArchiveRelationship(e.Admin(), edge.ID, nil); err != nil {
				t.Fatal(err)
			}
			after, err := service.CachedCards(reader, []ids.UUID{deal.UUID})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := after[deal.UUID]; ok {
				t.Fatal("the earlier assessment survived loss of its stakeholder context")
			}
		})
	}
}
