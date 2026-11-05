// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestAnEarlierAssessmentWithholdsMoneyAfterAFieldMaskChanges(t *testing.T) {
	for _, change := range []string{"mask", "owner"} {
		t.Run(change, func(t *testing.T) {
			e := integration.Setup(t)
			pipeline, stage, _ := integration.DealFixture(t, e)
			deal := ids.From[ids.DealKind](e.SeedDeal(t, "Demo renewal", pipeline, stage, &e.Rep1))
			store := e.Deals
			amount, currency := int64(48000000), "EUR"
			if _, err := store.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{AmountMinor: &amount, Currency: &currency}); err != nil {
				t.Fatal(err)
			}
			at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
			subject, occurred := "Renewal scope", at.Add(-time.Hour)
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: "call", Subject: &subject, Source: "manual", OccurredAt: &occurred,
				Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal.UUID}},
			})
			if err != nil {
				t.Fatal(err)
			}
			sawAmount := false
			lane := brainFunc(func(_ context.Context, request model.Request) (model.Response, error) {
				for _, message := range request.Messages {
					sawAmount = sawAmount || strings.Contains(message.Content, "48000000 EUR")
				}
				return model.Response{Text: fmt.Sprintf(`{"story":[{"text":"The renewal was discussed.","evidence":[%q]}],"blocker":[],"buyer":[],"verdict":{"standing":"live","because":[{"text":"The renewal is worth 480000 EUR.","evidence":[%q]}]},"move_reason":[]}`, source.Id.String(), source.Id.String())}, nil
			})
			permissions := integration.AdminPerms
			permissions.RoleKeys, permissions.RowScope = []string{"fixture"}, principal.RowScopeOwn
			if change == "owner" {
				permissions.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority}}
			}
			reader := e.As(e.Rep1, []ids.UUID{e.Team1}, permissions)
			service := newDealStatusService(e.Pool, func() time.Time { return at }).WithLane(lane, "test")
			card, err := service.Get(reader, deal, true)
			if err != nil || !sawAmount || card.GeneratedBy != crmcontracts.WrittenByModel || card.Verdict == nil {
				t.Fatalf("unmasked actual model input missing: saw=%t card=%+v err=%v", sawAmount, card, err)
			}
			before, err := service.CachedCards(reader, []ids.UUID{deal.UUID})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := before[deal.UUID]; !ok {
				t.Fatal("unmasked saved assessment was never served")
			}
			if change == "owner" {
				owner := ids.From[ids.UserKind](e.Rep3)
				if _, err := store.UpdateDeal(e.Admin(), deal, deals.UpdateDealInput{OwnerID: &owner}); err != nil {
					t.Fatal(err)
				}
			} else {
				permissions.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}}
				reader = e.As(e.Rep1, []ids.UUID{e.Team1}, permissions)
			}
			after, err := service.CachedCards(reader, []ids.UUID{deal.UUID})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := after[deal.UUID]; ok {
				t.Fatal("masked money survived in saved model text")
			}
		})
	}
}
