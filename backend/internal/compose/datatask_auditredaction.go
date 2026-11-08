// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type auditRedactionPage func(context.Context, pgx.Tx, ids.UUID, ids.UUID, int, privacy.CountryReader) (privacy.HistoricalAuditPage, error)

func auditRedactionDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1794066141_erased_audit_payloads", Version: 2,
		Title:    "Removing retained payloads from earlier privacy erasures",
		Requires: []string{"core/1794066141", "core/1794067376"}, Steps: auditRedactionSteps(privacy.RedactHistoricalAuditPageTx),
	}
}

func linkedAuditRedactionDataTask() datatask.Task {
	steps := auditRedactionSteps(privacy.RedactLinkedHistoricalAuditPageTx)
	identities := auditRedactionSteps(privacy.RedactLinkedHistoricalIdentitiesPageTx)[0]
	identities.Name = "request_identity_images"
	steps = append(steps, identities)
	return datatask.Task{ID: "core/1794076293_linked_erased_audit_payloads", Version: 1, Title: "Removing linked payloads left by earlier privacy erasures", Requires: []string{"core/1794066141", "core/1794067376", "core/1794076293"}, Steps: steps}
}

func markedAuditRedactionDataTask() datatask.Task {
	weekly := auditRedactionSteps(privacy.RedactMarkedWeeklyAuditPageTx)[0]
	weekly.Name = "marked_weekly_images"
	weekly.Limit = privacy.MarkerAuditPageSize
	requests := auditRedactionSteps(privacy.RedactRetiredRequestAuditPageTx)[0]
	requests.Name = "retired_request_images"
	requests.Limit = privacy.MarkerAuditPageSize
	return datatask.Task{ID: "core/1794076293_marked_erased_audit_payloads", Version: 1, Title: "Removing copies of already-cleared weekly items and privacy requests", Requires: []string{"core/1794066141", "core/1794067376", "core/1794076293"}, Steps: []datatask.Step{weekly, requests}}
}

func auditRedactionSteps(correct auditRedactionPage) []datatask.Step {
	return []datatask.Step{{
		Name: "audit_payloads", Limit: 200, MinLimit: 1,
		Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := privacy.CountHistoricalAuditRows(ctx, q, maximum)
			return min(count, maximum), count > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			page, err := correct(ctx, tx, after, in.RunID, in.Limit, identity.CountryForDecision)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
		},
	}}
}
