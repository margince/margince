// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestClosingAndReopeningPreviewTheWritersFrozenValues(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	pipeline, open, won := integration.DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Harbor renewal", pipeline, open, &e.Rep1))
	var currency string
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		currency, err = DealsInstallation().BaseCurrency(ctx, tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Deals.UpdateDeal(ctx, deal, deals.UpdateDealInput{AmountMinor: new(int64(250000)), Currency: &currency}); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC) }
	e.Deals.WithClock(now)
	reader := newApprovalPreviewReader(e.DB())
	reader.deals.WithClock(now)
	svc := approvals.NewService(e.DB()).WithPreview(reader.read)
	for _, destination := range []ids.StageID{won, open} {
		raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"deal_id":%q,"to_stage_id":%q,"won_without_contract_reason":"verbal"}`, deal, destination)))
		if err != nil {
			t.Fatal(err)
		}
		id, err := svc.Stage(ctx, approvals.StageInput{Kind: "advance_deal", TargetType: "deal", TargetID: deal.UUID, CoTargetType: "stage", CoTargetID: destination.UUID, ProposedChange: raw, DiffHash: hash})
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := svc.GetWire(ctx, id)
		if err != nil || proposal.Preview == nil || proposal.Preview.State != "ready" {
			t.Fatalf("preview=%+v, %v", proposal.Preview, err)
		}
		if destination == won && (proposal.Preview.Effects == nil || len(*proposal.Preview.Effects) != 2) {
			t.Fatalf("win effects=%+v", proposal.Preview.Effects)
		}
		if destination == open && proposal.Preview.Effects != nil && len(*proposal.Preview.Effects) != 0 {
			t.Fatalf("reopen effects=%+v", proposal.Preview.Effects)
		}
		seen := map[string]bool{}
		for _, change := range proposal.Preview.Changes {
			switch change.Field {
			case "closed_at", "fx_rate_to_base", "fx_rate_date", "amount_minor_base":
				seen[change.Field] = true
				if destination == won && (change.Before.Kind != "empty" || change.After.Kind != "execution") {
					t.Fatalf("closing effect=%+v", change)
				}
				if destination == open && change.Field == "amount_minor_base" && (change.Before.Kind != "money" || change.Before.Currency == nil || *change.Before.Currency != currency) {
					t.Fatalf("base amount=%+v", change.Before)
				}
				if destination == open && (change.Before.Kind == "empty" || change.After.Kind != "empty") {
					t.Fatalf("reopening effect=%+v", change)
				}
			}
		}
		if len(seen) != 4 {
			t.Fatalf("missing frozen effects: %v", seen)
		}
		if _, err := e.Deals.AdvanceDeal(ctx, deal, deals.AdvanceDealInput{ToStageID: destination, WonWithoutContractReason: new("verbal")}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRepricingAndReopeningKeepTheActualFrozenAuditBeforeImage(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	pipeline, open, won := integration.DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Harbor renewal", pipeline, open, &e.Rep1))
	var currency string
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		currency, err = DealsInstallation().BaseCurrency(ctx, tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Deals.UpdateDeal(ctx, deal, deals.UpdateDealInput{AmountMinor: new(int64(250000)), Currency: &currency}); err != nil {
		t.Fatal(err)
	}
	e.Deals.WithClock(func() time.Time { return time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC) })
	if _, err := e.Deals.AdvanceDeal(ctx, deal, deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: new("verbal")}); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"reprice", "reopen"} {
		var rate, date, amount string
		if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT fx_rate_to_base::text, fx_rate_date::text, amount_minor_base::text FROM deal WHERE id = @id`, pgx.StrictNamedArgs{"id": deal}).Scan(&rate, &date, &amount)
		}); err != nil {
			t.Fatal(err)
		}
		if operation == "reprice" {
			if _, err := e.Deals.UpdateDeal(ctx, deal, deals.UpdateDealInput{AmountMinor: new(int64(300000))}); err != nil {
				t.Fatal(err)
			}
		} else if _, err := e.Deals.AdvanceDeal(ctx, deal, deals.AdvanceDealInput{ToStageID: open}); err != nil {
			t.Fatal(err)
		}
		var beforeRate, beforeDate, beforeAmount string
		if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT before ->> 'fx_rate_to_base', before ->> 'fx_rate_date', before ->> 'amount_minor_base' FROM audit_log WHERE entity_type = 'deal' AND entity_id = @id AND after ? 'amount_minor_base' ORDER BY occurred_at DESC, id DESC LIMIT 1`, pgx.StrictNamedArgs{"id": deal}).Scan(&beforeRate, &beforeDate, &beforeAmount)
		}); err != nil {
			t.Fatal(err)
		}
		if beforeRate != rate || beforeDate != date || beforeAmount != amount {
			t.Fatalf("%s audit before: %s/%s/%s, stored: %s/%s/%s", operation, beforeRate, beforeDate, beforeAmount, rate, date, amount)
		}
	}
}

func TestALostReasonIsClearedOnEveryLandingThatIsNotLost(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	pipeline, open, won := integration.DealFixture(t, e)
	config, err := e.Deals.DefaultPipeline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var lost ids.StageID
	for _, stage := range *config.Stages {
		if stage.Semantic == "lost" {
			lost = ids.From[ids.StageKind](ids.UUID(stage.Id))
		}
	}
	if lost.IsZero() {
		t.Fatal("fixture has no lost stage")
	}
	for _, destination := range []ids.StageID{won, open} {
		deal := ids.From[ids.DealKind](e.SeedDeal(t, "Harbor renewal", pipeline, open, &e.Rep1))
		if _, err := e.Deals.AdvanceDeal(ctx, deal, deals.AdvanceDealInput{ToStageID: lost, LostReasonCode: new("competitor")}); err != nil {
			t.Fatal(err)
		}
		landed, err := e.Deals.AdvanceDeal(ctx, deal, deals.AdvanceDealInput{ToStageID: destination, WonWithoutContractReason: new("verbal")})
		if err != nil {
			t.Fatal(err)
		}
		if landed.LostReason != nil {
			t.Fatalf("loss reason survived: %+v", landed.LostReason)
		}
	}
}
