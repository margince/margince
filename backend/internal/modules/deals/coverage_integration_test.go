// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

type failingCatalog struct{}

func (failingCatalog) ActiveColumns(context.Context, string) ([]fieldcatalog.Column, error) {
	return nil, errors.New("catalog down")
}

// A transaction-held read refuses a caller without deal:read, and a deal that
// is not there. An empty outside id is not a filter. The erased and merged
// events stage in the writing transaction.
func TestDealTxReadsRefuseWhatTheyMustAndTheEventsStage(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.asDealWriter()
	if err := createSourcedDeal(ctx, t, e, "Renewal", freshSystem(), "D-1"); err != nil {
		t.Fatal(err)
	}
	listed, _, err := e.store.ListDeals(ctx, ListDealsInput{})
	if err != nil || len(listed) == 0 {
		t.Fatalf("ListDeals = %d, %v", len(listed), err)
	}
	id := ids.From[ids.DealKind](ids.UUID(listed[0].Id))

	if err := e.store.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := e.store.GetDealTx(e.asDealless(), tx, id, storekit.LiveOnly, CustomColumns{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("GetDealTx without deal:read = %v, want a permission refusal", err)
		}
		if _, err := e.store.GetDealTx(ctx, tx, ids.New[ids.DealKind](), storekit.LiveOnly, CustomColumns{}); err == nil {
			t.Error("GetDealTx of a missing deal succeeded")
		}
		auditID, err := storekit.AuditEvent(ctx, tx, "erase", "deal", id.UUID, nil)
		if err != nil {
			return err
		}
		if err := EmitDealErased(ctx, tx, auditID, id); err != nil {
			t.Errorf("EmitDealErased: %v", err)
		}
		if err := EmitDealMerged(ctx, tx, auditID, id, ids.New[ids.DealKind]()); err != nil {
			t.Errorf("EmitDealMerged: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	empty := ""
	if _, _, err := e.store.ListDeals(ctx, ListDealsInput{SourceID: &empty}); err == nil {
		t.Error("an empty source_id filter was admitted")
	}
	if _, err := DealColumnsFrom(ctx, failingCatalog{}); err == nil {
		t.Error("DealColumnsFrom swallowed a catalog failure")
	}
}
