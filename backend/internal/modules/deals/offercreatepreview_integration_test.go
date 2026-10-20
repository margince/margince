// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"errors"
	"maps"
	"math"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestOfferCreationPreviewMatchesProductSnapshotsAndTotals(t *testing.T) {
	e := setupSeamEnv(t)
	_, stage, _ := twoStagePipeline(t, e.configEnv)
	_, pipeline, err := e.provider.StageSemantic(e.passport(), stage.UUID)
	if err != nil {
		t.Fatal(err)
	}
	deal := e.create(t, datasource.EntityDeal, `{"name":"Renewal","pipeline_id":"`+pipeline.String()+`","stage_id":"`+stage.String()+`"}`)
	product := e.create(t, datasource.EntityProduct, `{"name":"Support","unit":"hour","unit_price_minor":10000,"currency":"EUR","default_tax_rate":19,"billing_model":"recurring","billing_interval_months":3}`)
	productID := ids.From[ids.ProductKind](product.ID)
	input := CreateOfferInput{Currency: "EUR", Source: "manual", LineItems: []OfferLineInputRow{
		{Position: new(4), ProductID: &productID, Quantity: "2", DiscountPct: new("10"), IntervalCount: new(2)},
		{Position: new(1), Description: new("Setup"), Quantity: "1", UnitPriceMinor: new(int64(500))},
		{Description: new("Warranty"), Quantity: "1", UnitPriceMinor: new(int64(0))},
	}}
	store := e.provider.store
	dealID := ids.From[ids.DealKind](deal.ID)
	plan, err := store.PreviewOfferCreation(e.passport(), dealID, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Lines) != 3 || plan.Totals.NetMinor != 18500 || plan.Totals.TaxMinor != 3420 || plan.Totals.GrossMinor != 21920 {
		t.Fatalf("incorrect plan: %+v", plan)
	}
	var count int
	if err := e.owner.QueryRow(t.Context(), `SELECT count(*) FROM offer WHERE deal_id=$1`, dealID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("preview wrote %d offers", count)
	}
	actor, ok := principal.Actor(e.passport())
	if !ok {
		t.Fatal("missing fixture actor")
	}
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	delete(actor.Permissions.Objects, "product")
	denied, err := store.PreviewOfferCreation(principal.WithActor(e.passport(), actor), dealID, input)
	if !errors.Is(err, apperrors.ErrPermissionDenied) || len(denied.Lines) != 0 {
		t.Fatalf("product snapshot disclosed: %+v / %v", denied, err)
	}
	actual, err := store.CreateOffer(e.passport(), dealID, input)
	if err != nil {
		t.Fatal(err)
	}
	if actual.NetMinor == nil || *actual.NetMinor != plan.Totals.NetMinor || actual.GrossMinor == nil || *actual.GrossMinor != plan.Totals.GrossMinor || actual.LineItems == nil || len(*actual.LineItems) != len(plan.Lines) {
		t.Fatalf("writer differs: %+v", actual)
	}
	if actual.ArrMinor == nil || *actual.ArrMinor != plan.Recurring.ArrMinor || actual.NetTcvMinor == nil || *actual.NetTcvMinor != plan.Recurring.NetTcvMinor {
		t.Fatalf("recurring totals differ: %+v / %+v", actual, plan.Recurring)
	}
	for _, expected := range plan.Lines {
		found := false
		for _, line := range *actual.LineItems {
			if line.Position != *expected.Input.Position {
				continue
			}
			found = true
			if line.Description != *expected.Input.Description || line.Unit != *expected.Input.Unit || line.UnitPriceMinor != *expected.Input.UnitPriceMinor {
				t.Fatalf("line differs: %+v / %+v", line, expected)
			}
		}
		if !found {
			t.Fatalf("missing position: %d", *expected.Input.Position)
		}
	}
	if input.LineItems[0].Description != nil || input.LineItems[2].Position != nil {
		t.Fatal("preview mutated request inputs")
	}
}

func TestOfferCreationPreviewRefusesCollidingOrOverflowedPositions(t *testing.T) {
	e := setupSeamEnv(t)
	_, stage, _ := twoStagePipeline(t, e.configEnv)
	_, pipeline, err := e.provider.StageSemantic(e.passport(), stage.UUID)
	if err != nil {
		t.Fatal(err)
	}
	deal := e.create(t, datasource.EntityDeal, `{"name":"Positions","pipeline_id":"`+pipeline.String()+`","stage_id":"`+stage.String()+`"}`)
	for _, first := range []int{1, math.MaxInt32} {
		input := CreateOfferInput{Currency: "EUR", Source: "manual", LineItems: []OfferLineInputRow{
			{Position: &first, Description: new("First"), Quantity: "1", UnitPriceMinor: new(int64(100))},
			{Description: new("Second"), Quantity: "1", UnitPriceMinor: new(int64(100))},
		}}
		if first == 1 {
			input.LineItems[1].Position = new(1)
		}
		_, previewErr := e.provider.store.PreviewOfferCreation(e.passport(), ids.From[ids.DealKind](deal.ID), input)
		_, writerErr := e.provider.store.CreateOffer(e.passport(), ids.From[ids.DealKind](deal.ID), input)
		if previewErr == nil || writerErr == nil {
			t.Fatalf("invalid positions admitted: preview=%v write=%v", previewErr, writerErr)
		}
		if first == 1 && (!errors.Is(previewErr, apperrors.ErrConflict) || !errors.Is(writerErr, apperrors.ErrConflict)) {
			t.Fatalf("wrong collision error: %v / %v", previewErr, writerErr)
		}
		if first == math.MaxInt32 {
			input.LineItems = input.LineItems[:1]
			offer, err := e.provider.store.CreateOffer(e.passport(), ids.From[ids.DealKind](deal.ID), input)
			if err != nil {
				t.Fatal(err)
			}
			offerID := ids.From[ids.OfferKind](ids.UUID(offer.Id))
			_, appendErr := e.provider.store.AddOfferLineItem(e.passport(), offerID, OfferLineInputRow{Description: new("Append"), Quantity: "1", UnitPriceMinor: new(int64(100))})
			_, stagedErr := e.provider.store.AddStagedOfferLines(e.passport(), offerID, []StagedOfferLineInput{{}})
			for _, err := range []error{appendErr, stagedErr} {
				var figure *FigureError
				if !errors.As(err, &figure) || figure.Field != "position" {
					t.Fatalf("append overflow reached storage: %v", err)
				}
			}
		}

	}
}
