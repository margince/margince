// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func linkedInIdentityDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1793062663_linkedin_identity_normalized", Version: 1,
		Title: "Normalize stored LinkedIn identities", Requires: []string{"core/0001"},
		Steps: []datatask.Step{
			normalizeLinkedInStep("normalize_leads", "lead", contacts.NormalizeLeadLinkedInTx),
			normalizeLinkedInStep("normalize_contacts", "contact", contacts.NormalizeContactLinkedInTx),
		},
	}
}

func normalizeLinkedInStep(
	name, entity string,
	normalize func(context.Context, pgx.Tx, ids.UUID, int) ([]ids.UUID, int, error),
) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 200, MinLimit: 10, Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := contacts.CountStoredLinkedIn(ctx, q, entity, maximum)
			return min(count, maximum), count > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			if err := auth.RequireSystem(ctx); err != nil {
				return datatask.BatchOut{}, err
			}
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			inspected, changed, err := normalize(ctx, tx, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, inspected, changed)
		},
	}
}

func leadContactPairsDataTask(pool *pgxpool.Pool) datatask.Task {
	db := InstallationDB(pool)
	consents := consent.NewStore(db)
	store := newContactsStore(db).WithStopCarrier(consents).WithSatelliteCarriers(consents, nil)
	return datatask.Task{
		ID: "core/1793062664_lead_contact_pairs_merged", Version: 1,
		Title: "Merge existing lead and contact pairs", Requires: []string{"core/0001"},
		Steps: []datatask.Step{{
			Name: "merge_pairs", Limit: 25, MinLimit: 1, EmptyWhenDone: true, Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
					return 0, false, err
				}
				count, err := contacts.CountExistingLeadContactPairs(ctx, q, maximum)
				return min(count, maximum), count > maximum, err
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				page, err := store.MergeExistingLeadContactPairsTx(ctx, tx, after, in.Limit)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				for _, leadID := range page.Ambiguous {
					slog.WarnContext(ctx, "lead matches more than one contact during identity cleanup",
						"lead_id", leadID)
				}
				return datatask.AdvanceUUID(in, page.Inspected, page.Merged)
			},
		}},
	}
}
