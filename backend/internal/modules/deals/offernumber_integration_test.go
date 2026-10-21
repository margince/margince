// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// The offer number is the one number this product mints for a customer. A
// mint under a snapshot older than its lock could hand out a number twice.
// A row in another shape would let the next mint reuse one.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func offerNumberDeal(t *testing.T, e *seamEnv) ids.DealID {
	t.Helper()
	_, stage, _ := twoStagePipeline(t, e.configEnv)
	_, pipeline, err := e.provider.StageSemantic(e.passport(), stage.UUID)
	if err != nil {
		t.Fatal(err)
	}
	deal := e.create(t, datasource.EntityDeal, `{"name":"Renewal","pipeline_id":"`+pipeline.String()+`","stage_id":"`+stage.String()+`"}`)
	return ids.From[ids.DealKind](deal.ID)
}

// A snapshot taken before the mint's lock reads a stale highest number, so the
// mint refuses one rather than risk issuing a number twice.
func TestAnOfferIsNotMintedUnderARepeatableReadSnapshot(t *testing.T) {
	e := setupSeamEnv(t)
	deal := offerNumberDeal(t, e)
	input := CreateOfferInput{Currency: "EUR", Source: "manual"}

	err := database.WithWorkspaceSnapshot(e.passport(), e.pool, func(ctx context.Context) error {
		_, err := e.provider.store.CreateOffer(ctx, deal, input)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "read committed") {
		t.Fatalf("minting under a repeatable-read snapshot: err = %v, want it refused", err)
	}
	created, err := e.provider.store.CreateOffer(e.passport(), deal, input)
	if err != nil || created.OfferNumber == nil || !strings.HasPrefix(*created.OfferNumber, "A-") {
		t.Fatalf("minting at read committed = (%v, %v), want an A- number", created.OfferNumber, err)
	}
}

// A number the mint could not have issued is refused by the database itself.
// The mint would not see it, and would reissue the number after it.
func TestAnOfferNumberInAnotherShapeIsRefused(t *testing.T) {
	e := setupSeamEnv(t)
	deal := offerNumberDeal(t, e)

	_, err := e.owner.Exec(t.Context(), `
		INSERT INTO offer (deal_id, offer_number, currency, source, captured_by)
		VALUES ($1, 'AN-2026-001', 'EUR', 'manual', 'human:test')`, deal)
	if err == nil || !strings.Contains(err.Error(), "offer_number_shape") {
		t.Fatalf("an offer numbered AN-2026-001: err = %v, want offer_number_shape to refuse it", err)
	}
}
