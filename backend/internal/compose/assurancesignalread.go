// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// assuranceExceptionsOwnedBy is the one read every surface that shows findings
// goes through: the list endpoint, list_input_checks and the weekly digest. It
// is also where each finding learns whether its inputs moved since the check,
// so no surface can show a finding without that answer.
func assuranceExceptionsOwnedBy(ctx context.Context, tx pgx.Tx, owners []ids.UUID, requested *RequestedScope, limit int) ([]assurance.Exception, error) {
	found, err := assuranceDealExceptionsOwnedBy(ctx, tx, owners, requested, 0)
	if err != nil {
		return nil, err
	}
	signals, err := assuranceSignalExceptions(ctx, tx, owners, requested)
	if err != nil {
		return nil, err
	}
	found = append(found, signals...)
	calls, err := assuranceCallExceptions(ctx, tx, owners, requested)
	if err != nil {
		return nil, err
	}
	found = append(found, calls...)
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].Severity != found[j].Severity {
			return assuranceSeverity(found[i].Severity) < assuranceSeverity(found[j].Severity)
		}
		if !found[i].LastSeenAt.Equal(found[j].LastSeenAt) {
			return found[i].LastSeenAt.After(found[j].LastSeenAt)
		}
		return found[i].ID.String() < found[j].ID.String()
	})
	if limit > 0 && len(found) > limit {
		found = found[:limit]
	}
	if err := assurance.MarkOutOfDate(ctx, tx, found); err != nil {
		return nil, err
	}
	return found, nil
}

func assuranceSeverity(severity string) int {
	switch severity {
	case assurance.SeverityHigh:
		return 0
	case assurance.SeverityMedium:
		return 1
	default:
		return 2
	}
}

func assuranceSignalExceptions(ctx context.Context, tx pgx.Tx, owners []ids.UUID, requested *RequestedScope) ([]assurance.Exception, error) {
	if !auth.ReadGranted(ctx, "signal") || !auth.ReadGranted(ctx, "activity") || !auth.ReadGranted(ctx, "company") {
		return nil, nil
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	signalScope, err := auth.SignalScopeClause(ctx, "signal_subject", arg)
	if err != nil {
		return nil, err
	}
	if signalScope == "" {
		signalScope = sqlUnnarrowed
	}
	companyScope, err := auth.ScopeClauseFor(ctx, "company", "c", arg)
	if err != nil {
		return nil, err
	}
	if companyScope != "" {
		signalScope += " AND " + companyScope
	}
	// The content gate runs once, over every activity an open finding's signal
	// or its suggestion's evidence cites. Each copy is planned separately, and
	// the planner's memory stays with the pooled connection.
	open := `SELECT %[1]s FROM assurance_exception oe WHERE oe.subject_kind = 'signal' AND oe.status = 'open'`
	readable, err := auth.ContentReadableActivitiesSQL(ctx, `SELECT ra.id FROM activity ra WHERE ra.archived_at IS NULL AND ra.id IN (
   SELECT `+storekit.CitedActivityID("cite")+` FROM signal os, jsonb_array_elements(os.evidence) cite
    WHERE cite->>'source_type' = 'activity' AND os.id IN (`+fmt.Sprintf(open, "oe.subject_id")+`)
   UNION ALL `+deals.SuggestionEvidenceActivitiesSQL(fmt.Sprintf(open, "oe.observed->>'suggestion_id'"))+`)`, arg)
	if err != nil {
		return nil, err
	}
	if owners != nil {
		signalScope += fmt.Sprintf(" AND c.owner_id = ANY($%d)", arg(owners))
	}
	if requested != nil {
		_, population, err := AnalyticsPopulationClause(ctx, tx, *requested, "c", arg, unownedIsPartOfDefault)
		if err != nil {
			return nil, err
		}
		if population != "" {
			signalScope += " AND " + population
		}
	}
	suggestionScope, err := deals.ReadableSuggestionClauseWithin(ctx, arg, "readable_activity")
	if err != nil {
		return nil, err
	}
	columns := strings.Replace(exceptionSelect(rawFindingMoney), rawFindingMoney.observed, `CASE WHEN EXISTS (
  SELECT 1 FROM deal_suggestion s WHERE s.id::text = e.observed->>'suggestion_id'
  AND s.state = 'open' AND `+suggestionScope+`) THEN e.observed ELSE e.observed - 'suggestion_id' END`, 1)
	rows, err := tx.Query(ctx, `WITH readable_activity AS MATERIALIZED (`+readable+`)
 SELECT `+columns+`, c.display_name, c.owner_id
 FROM assurance_exception e JOIN signal signal_subject ON signal_subject.id = e.subject_id
 JOIN company c ON c.id = signal_subject.resolved_company_id
 WHERE e.subject_kind = 'signal' AND e.status = 'open'
 AND signal_subject.archived_at IS NULL AND c.archived_at IS NULL AND `+signalScope+`
 AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(signal_subject.evidence) cite
  WHERE cite->>'source_type' = 'activity' AND NOT EXISTS (SELECT 1 FROM readable_activity ca
   WHERE ca.id = `+storekit.CitedActivityID("cite")+`))
 ORDER BY e.last_seen_at DESC, e.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading signal findings: %w", err)
	}
	return pgx.CollectRows(rows, scanLabelledException)
}
