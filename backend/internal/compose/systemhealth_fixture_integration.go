// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/continuity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/pkg/extension"
)

// SeedSystemHealthFixture uses production writers so benchmarks read real workflow state.
func SeedSystemHealthFixture(ctx context.Context, pool *pgxpool.Pool, owner ids.UUID, ownerEmail string, count int, now time.Time) error {
	provider := providerGmail
	actorID := "connector:" + provider
	connectorCtx := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: actorID, UserID: owner,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			string(recordTypeActivity): {Create: true, Read: true, Update: true}, string(recordTypeContact): {Create: true, Read: true, Update: true}, string(recordTypeCompany): {Create: true, Read: true}, string(recordTypeLead): {Create: true, Read: true}, "installation_settings": {Read: true},
		}},
	})
	sink := newCaptureSink(pool, CaptureConfig{})
	for i := range count {
		source := fmt.Sprintf("health-fixture-%d", i)
		sender := fmt.Sprintf("sender-%d@health-%d.test", i, i)
		_, err := sink.Upsert(principal.WithCorrelationID(connectorCtx, ids.NewV7()), connector.NormalizedRecord{
			EntityType: datasource.EntityActivity, NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: source},
			Counterparty: connector.Counterparty{Email: sender, DisplayName: "Synthetic Sender", Domain: fmt.Sprintf("health-%d.test", i), Direction: connector.DirectionInbound},
			Fields:       capture.ActivityFields{Kind: string(crmcontracts.ActivityKindEmail), Subject: "Synthetic health evidence", Body: "Synthetic capture fixture", Direction: connector.DirectionInbound, OccurredAt: now.UTC()},
			ThreadKey:    source, Addresses: []string{sender, ownerEmail}, Source: provider + ":" + source, CapturedBy: actorID,
		})
		if err != nil {
			return fmt.Errorf("seeding capture health: %w", err)
		}
	}
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		classifier, err := captureClassifierHealth(ctx, tx, now.UTC())
		if err != nil {
			return err
		}
		if classifier.Pending+classifier.Unsure+classifier.Exhausted == 0 {
			var cause string
			if err := tx.QueryRow(ctx, `SELECT coalesce(max(detail->>'reason'),'none') FROM system_log WHERE action='capture_ensure_fault'`).Scan(&cause); err != nil {
				return err
			}
			return fmt.Errorf("capture benchmark produced no classifier backlog: %s", cause)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		for range count {
			if err := recordIngestRefusal(ctx, tx, "health_fixture", extension.RefusalKey, now.UTC()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	ledger := continuity.NewStore(InstallationDB(pool))
	drill, err := ledger.Begin(ctx, now.Add(-time.Minute), "Synthetic Operator", "")
	if err != nil {
		return err
	}
	return ledger.Finish(ctx, drill, continuity.OutcomePassed, "")
}
