// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// AssuranceSignalSubjects supplies source-grounded requests to the shared assurance scan.
func AssuranceSignalSubjects(ctx context.Context, tx pgx.Tx, now time.Time) ([]assurance.SignalSubject, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "signal", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := assuranceSignalSubjectsQuery(now, arg)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading unlinked buying signals: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (assurance.SignalSubject, error) {
		var subject assurance.SignalSubject
		err := row.Scan(&subject.SignalID, &subject.Owner, &subject.CompanyID, &subject.SuggestionID, &subject.OccurredAt, &subject.Historical, &subject.BuyerVerified)
		return subject, err
	})
}

// assuranceSignalSubjectsQuery is the statement AssuranceSignalSubjects runs.
// Rendered apart so a test can read which tables the check reads.
func assuranceSignalSubjectsQuery(now time.Time, arg func(any) int) string {
	return fmt.Sprintf(`SELECT s.id::text, coalesce(c.owner_id::text, ''), c.id::text,
  coalesce(covered.id::text, ''), source.occurred_at, source.occurred_at < $%[1]d, %[3]s -- occurred_at raw: a signal's cited time is placed already
 FROM signal s JOIN company c ON c.id = s.resolved_company_id
 CROSS JOIN LATERAL (SELECT `+storekit.CitedActivityTime("s.evidence")+` AS occurred_at) source
 LEFT JOIN LATERAL (SELECT ds.id FROM deal_suggestion ds
  JOIN deal_suggestion_evidence ev ON ev.suggestion_id = ds.id AND ev.signal_id = s.id
  WHERE ds.state = 'open' ORDER BY ds.created_at, ds.id LIMIT 1) covered ON true
 WHERE s.kind = 'new_opportunity' AND s.status = 'open' AND s.archived_at IS NULL
 AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(s.evidence) role_cite
 WHERE role_cite->>'commercial_role' IN ('supplier', 'partner', 'provider'))
 AND (source.occurred_at >= $%[1]d OR %[3]s) -- occurred_at raw: a signal's cited time is placed already
 AND s.visibility = 'workspace' AND c.archived_at IS NULL AND NOT c.is_anchor
 AND source.occurred_at <= $%[2]d -- occurred_at raw: a signal's cited time is placed already
 AND coalesce(`+deals.SuggestionFloorExpr("c.id")+`, '-infinity'::timestamptz) < source.occurred_at -- occurred_at raw: past the suggestion floor, a window's lower edge
 AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(s.evidence) cite
  WHERE cite->>'source_type' = 'activity' AND NOT EXISTS (SELECT 1 FROM activity ca
   WHERE ca.id = `+storekit.CitedActivityID("cite")+` AND `+workspaceEvidence("ca")+`))
 AND `+unlinkedCommercialMatter("s")+`
 ORDER BY s.id`, arg(now.Add(-dealScoutWindow)), arg(now), scoutBuyerSignal("s"))
}
