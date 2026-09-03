// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The findings one caller may see.
//
// The list goes through the deal's own row scope rather than reading
// assurance_exception directly. On a deal that predicate renders TRUE — deals
// are an IDENTITY table, workspace-readable by every seat, and what keeps one
// the owner's is the write arm rather than the read arm (auth/tableclass.go).
// So today the scope narrows nothing here.
//
// The join is what makes that a DECISION rather than an accident. A finding
// reaches a reader only if the deal it is about does, so the day a record type
// arrives scoped — or a deal stops being workspace-readable — this list narrows
// with it instead of quietly outliving the rule. Reading the exception table
// alone would leave a second answer to "who may see this deal" that nothing
// keeps in step.
//
// It lives in compose rather than in the assurance module because the clause is
// over `deal`, which the module does not own and may not import.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AssuranceExceptions reads the open findings this caller may see.
//
// The scope is applied in SQL rather than by filtering afterwards. Filtering a
// full list in Go means the rows crossed a boundary they should not have, and
// the count of what was dropped is itself a statement about how much there is.
func AssuranceExceptions(ctx context.Context, tx pgx.Tx) ([]assurance.Exception, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	scopeClause, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
	if err != nil {
		return nil, err
	}
	if scopeClause == "" {
		scopeClause = sqlUnnarrowed
	}

	// An INNER join to deal, so a finding whose subject is not a deal — or is a
	// deal that has since been archived away — does not arrive without a
	// visibility decision having been made about it. A LEFT join would let
	// exactly the rows nobody can gate through.
	sql := fmt.Sprintf(`
		SELECT e.id, e.type, e.subject_kind, e.subject_id, e.severity,
		       e.affected_minor, e.currency, e.owner_id, e.status,
		       e.claim, e.observed, e.first_seen_at, e.last_seen_at
		FROM assurance_exception e
		JOIN deal d ON d.id = e.subject_id
		WHERE e.status = 'open'
		  AND e.subject_kind = 'deal'
		  AND d.archived_at IS NULL
		  AND %s
		ORDER BY
		  -- Material first: the manager reading this before a call has minutes,
		  -- and a high-severity finding on a large deal is what those minutes
		  -- are for.
		  CASE e.severity WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END,
		  e.affected_minor DESC NULLS LAST,
		  e.last_seen_at DESC`, scopeClause)

	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the findings: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (assurance.Exception, error) {
		var out assurance.Exception
		var id, subjectID ids.UUID
		var owner *ids.UUID
		var currency *string
		var firstSeen, lastSeen time.Time
		err := row.Scan(&id, &out.Type, &out.SubjectKind, &subjectID, &out.Severity,
			&out.AffectedMinor, &currency, &owner, &out.Status,
			&out.Claim, &out.Observed, &firstSeen, &lastSeen)
		out.ID = id
		out.SubjectID = subjectID
		out.FirstSeenAt = firstSeen
		out.LastSeenAt = lastSeen
		if currency != nil {
			out.Currency = *currency
		}
		if owner != nil {
			out.OwnerID = owner
		}
		return out, err
	})
}
