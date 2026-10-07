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
		sources := []struct{ table, unit, budget, provider string }{
			{"site_read", "website_reads", contacts.BudgetDeferredSiteReads, contacts.ProviderDeferredSiteReads},
			{"company_scan", "account_scans", companyscan.BudgetDeferredScans, companyscan.ProviderDeferredScans},
			{"voice_build", "voice_builds", ai.BudgetDeferredVoiceBuilds, ai.ProviderDeferredVoiceBuilds},
		}
		out := make([]crmcontracts.AiDeferredWork, 0, len(sources))
		for _, source := range sources {
			line := crmcontracts.AiDeferredWork{Carrier: source.table, Unit: source.unit}
			err := InstallationDB(pool).Tx(ctx, func(tx pgx.Tx) error {
				var count, waiting int64
				if err := tx.QueryRow(ctx, "SELECT count(*) FILTER (WHERE "+source.budget+"), count(*) FILTER (WHERE "+source.provider+
					") FROM "+source.table+" WHERE ("+source.budget+") OR ("+source.provider+")").Scan(&count, &waiting); err != nil {
					return err
				}
				line.Count = &count
				line.WaitingOnProvider = &waiting
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
