// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/companyscan"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func aiDeferredWork(pool *pgxpool.Pool) func(context.Context) ([]crmcontracts.AiDeferredWork, error) {
	return func(ctx context.Context) ([]crmcontracts.AiDeferredWork, error) {
		if err := auth.Require(ctx, "ai_diagnostics", principal.ActionRead); err != nil {
			return nil, err
		}
		sources := []struct{ table, query, unit string }{
			{"site_read", "SELECT count(*) FROM site_read WHERE " + contacts.BudgetDeferredSiteReads, "website_reads"},
			{"company_scan", "SELECT count(*) FROM company_scan WHERE " + companyscan.BudgetDeferredScans, "account_scans"},
			{"voice_build", "SELECT count(*) FROM voice_build WHERE " + ai.BudgetDeferredVoiceBuilds, "voice_builds"},
		}
		out := make([]crmcontracts.AiDeferredWork, 0, len(sources))
		for _, source := range sources {
			line := crmcontracts.AiDeferredWork{Carrier: source.table, Unit: source.unit}
			err := InstallationDB(pool).Tx(ctx, func(tx pgx.Tx) error {
				var count int64
				if err := tx.QueryRow(ctx, source.query).Scan(&count); err != nil {
					return err
				}
				line.Count = &count
				line.Available = true
				return nil
			})
			if err != nil {
				slog.ErrorContext(ctx, "read deferred AI work", "carrier", source.table, "err", err)
			}
			out = append(out, line)
		}
		return out, nil
	}
}
