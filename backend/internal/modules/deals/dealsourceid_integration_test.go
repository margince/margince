// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// freshSystem names an outside system no other test wrote to: the unique pair
// spans the whole database, and the tests of one package share it.
func freshSystem() string { return "erp-" + ids.NewV7().String() }

func createSourcedDeal(ctx context.Context, t *testing.T, e *configEnv, name, system, sourceID string) error {
	t.Helper()
	stage := openStage(t, e, SemanticOpen)
	var pipeline ids.PipelineID
	if err := e.owner.QueryRow(t.Context(), `SELECT pipeline_id FROM stage WHERE id=$1`, stage).Scan(&pipeline); err != nil {
		t.Fatal(err)
	}
	_, err := e.store.CreateDeal(ctx, CreateDealInput{
		Name: name, PipelineID: pipeline, StageID: stage, Source: "manual",
		SourceSystem: &system, SourceID: &sourceID,
	})
	return err
}

func TestADealIsFoundByItsIDInTheOutsideSystem(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.asDealWriter()
	system := freshSystem()
	if err := createSourcedDeal(ctx, t, e, "Renewal 2027", system, "D-1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := createSourcedDeal(ctx, t, e, "Upsell", system, "D-2"); err != nil {
		t.Fatalf("create: %v", err)
	}

	found, _, err := e.store.ListDeals(ctx, ListDealsInput{SourceSystem: &system, SourceID: new("D-1")})
	if err != nil || len(found) != 1 || found[0].Name != "Renewal 2027" {
		t.Fatalf("lookup by (%s, D-1) = %d deals, err %v; want exactly the renewal", system, len(found), err)
	}
	other := freshSystem()
	none, _, err := e.store.ListDeals(ctx, ListDealsInput{SourceSystem: &other, SourceID: new("D-1")})
	if err != nil || len(none) != 0 {
		t.Fatalf("lookup in another system = %d deals, err %v; want an empty page", len(none), err)
	}
}

func TestTheSameOutsideIDCannotNameTwoDeals(t *testing.T) {
	e := setupConfigEnv(t)
	ctx := e.asDealWriter()
	system := freshSystem()
	if err := createSourcedDeal(ctx, t, e, "Renewal 2027", system, "D-1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := createSourcedDeal(ctx, t, e, "Rival", system, "D-1"); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("second deal with (%s, D-1) = %v, want a conflict", system, err)
	}
	if err := createSourcedDeal(ctx, t, e, "Rival", freshSystem(), "D-1"); err != nil {
		t.Fatalf("the same id in another system is a different pair: %v", err)
	}
}

// A deal is an identity table: every seat holding deal:read reads every deal,
// so the lookup's boundary is the object grant and not row scope.
func TestTheOutsideIDLookupOfDealsFollowsTheDealReadGrant(t *testing.T) {
	e := setupConfigEnv(t)
	system := freshSystem()
	if err := createSourcedDeal(e.asDealWriter(), t, e, "Renewal 2027", system, "D-1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	lookup := ListDealsInput{SourceSystem: &system, SourceID: new("D-1")}

	if _, _, err := e.store.ListDeals(e.asDealless(), lookup); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a reader holding no deal:read looked a deal up (%v)", err)
	}
	found, _, err := e.store.ListDeals(e.asScopedStranger(), lookup)
	if err != nil || len(found) != 1 {
		t.Fatalf("a colleague holding deal:read found %d deals, err %v; want 1", len(found), err)
	}
}
