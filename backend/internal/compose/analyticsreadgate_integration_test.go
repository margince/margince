// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestEveryAnalyticsReaderRequiresThePopulationGrant(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	e.seedOpenDeal(t, "Restricted population", 20, &e.Rep1, &amount, nil)
	ctx := e.reportReaderCtx()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("reader has no principal")
	}
	delete(actor.Permissions.Objects, "deal")
	ctx = principal.WithActor(ctx, actor)
	store := forecasting.NewStore(InstallationDB(e.Pool))
	query := analyticsquery.Query{
		Entity: "open-deals-per-company", Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}},
	}
	for _, reader := range []struct {
		name string
		read func(context.Context, pgx.Tx) error
	}{
		{"query", func(ctx context.Context, tx pgx.Tx) error {
			_, err := RunAnalyticsQuery(ctx, tx, query, analyticsquery.DefaultFloor)
			return err
		}},
		{"explanation", func(ctx context.Context, tx pgx.Tx) error {
			_, err := ExplainAnalyticsCell(ctx, tx, analyticsquery.Explain{Query: query}, analyticsquery.DefaultFloor)
			return err
		}},
		{"kept answer sources", func(ctx context.Context, tx pgx.Tx) error {
			_, _, err := analyticsSources(ctx, tx, query, nil)
			return err
		}},
	} {
		t.Run(reader.name, func(t *testing.T) {
			err := store.InTx(ctx, reader.read)
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("reader returned %v, want permission denied", err)
			}
		})
	}
	t.Run("schema", func(t *testing.T) {
		err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			schema, _, err := AnalyticsSchemaIn(ctx, tx)
			if err != nil {
				return err
			}
			if _, offered := schema.Entities[query.Entity]; offered {
				t.Error("schema offered the population without its read grant")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}
