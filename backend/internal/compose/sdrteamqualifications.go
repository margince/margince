// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type sdrQualifications struct {
	owners                          map[ids.UUID]sdrTeamLeads
	unattributed, monthUnattributed int
}

func readSdrTeamQualifications(ctx context.Context, tx pgx.Tx, seats []ids.UUID, window sdrTeamWindow, month *sdrTeamWindow, at time.Time) (sdrQualifications, error) {
	out := sdrQualifications{owners: map[ids.UUID]sdrTeamLeads{}}
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return out, err
	}
	var b reportingBindings
	scope, err := auth.ScopeClauseFor(ctx, string(recordTypeLead), "l", b.arg)
	if err != nil {
		return out, err
	}
	fields, _, err := reportingFieldScope(ctx, string(recordTypeLead), "l", reportingLeadFields(reportingLeadsQualified), &b)
	if err != nil {
		return out, err
	}
	inWindow := func(span sdrTeamWindow) string {
		return "qualified.qualified_at>=" + b.add(span.from) + " AND qualified.qualified_at<" + b.add(span.to)
	}
	inMonth, earliest := sqlNoRecord, window.from
	if month != nil {
		inMonth = inWindow(*month)
		if month.from.Before(earliest) {
			earliest = month.from
		}
	}
	query := `SELECT qualified.owner_id,count(*) FILTER (WHERE ` + inWindow(window) + `),count(*) FILTER (WHERE ` + inMonth + `),qualified.ownership_recorded
 FROM lead l ` + firstQualificationSQL(b.add(at)) + `
 WHERE ` + orUnnarrowed(scope) + ` AND ` + fields + ` AND qualified.qualified_at>=` + b.add(earliest) + `
 AND (qualified.owner_id=ANY(` + b.add(seats) + `) OR NOT qualified.ownership_recorded)
 GROUP BY qualified.owner_id,qualified.ownership_recorded`
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var owner *ids.UUID
		var counts sdrTeamLeads
		var recorded bool
		if err := rows.Scan(&owner, &counts.qualified, &counts.monthQualified, &recorded); err != nil {
			return out, err
		}
		if !recorded {
			out.unattributed += counts.qualified
			out.monthUnattributed += counts.monthQualified
		} else if owner != nil {
			out.owners[*owner] = counts
		}
	}
	return out, rows.Err()
}
