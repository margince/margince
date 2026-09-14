// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func demoteForRelationshipPatch(ctx context.Context, tx pgx.Tx, current relationshipRow, id ids.UUID, in UpdateRelationshipInput) error {
	if in.IsCurrentPrimary == nil || !*in.IsCurrentPrimary || current.Kind != employmentKind || current.ContactID == nil {
		return nil
	}
	var args []any
	arg := func(v any) string { args = append(args, v); return storekit.SQLf("$%d", len(args)) }
	contact, row := arg(*current.ContactID), arg(id)
	ended := storekit.SQLf("CASE WHEN %s::boolean THEN NULL ELSE coalesce(%s::date, patched.ended_at) END", arg(in.ClearEndedAt), arg(in.EndedAt))
	status := storekit.SQLf("coalesce(%s::text, patched.employment_status)", arg(in.EmploymentStatus))
	precision := storekit.SQLf("coalesce(%s::text, patched.ended_precision)", arg(in.EndedPrecision))
	_, err := tx.Exec(ctx, storekit.SQLf(`UPDATE relationship SET is_current_primary=false
 WHERE contact_id=%s AND id<>%s AND %s
 AND EXISTS(SELECT 1 FROM relationship patched WHERE patched.id=%s AND %s)`,
		contact, row, employment.CurrentPrimarySlotSQL(""), row, employment.IsCurrentSQL(ended, status, precision)), args...)
	return err
}

func patchRelationshipRow(ctx context.Context, tx pgx.Tx, id ids.UUID, in UpdateRelationshipInput, capturedBy string) (relationshipRow, error) {
	var args []any
	arg := func(v any) string { args = append(args, v); return storekit.SQLf("$%d", len(args)) }
	row, role, primary := arg(id), arg(in.Role), arg(in.IsCurrentPrimary)
	started := storekit.SQLf("CASE WHEN %s::boolean THEN NULL ELSE coalesce(%s::date, started_at) END", arg(in.ClearStartedAt), arg(in.StartedAt))
	ended := storekit.SQLf("CASE WHEN %s::boolean THEN NULL ELSE coalesce(%s::date, ended_at) END", arg(in.ClearEndedAt), arg(in.EndedAt))
	status := storekit.SQLf("coalesce(%s::text, employment_status)", arg(in.EmploymentStatus))
	startPrecision := storekit.SQLf("CASE WHEN %s::boolean THEN NULL ELSE coalesce(%s::text, started_precision) END", arg(in.ClearStartedAt), arg(in.StartedPrecision))
	endPrecision := storekit.SQLf("CASE WHEN %s::boolean THEN NULL ELSE coalesce(%s::text, ended_precision) END", arg(in.ClearEndedAt), arg(in.EndedPrecision))
	captured := arg(capturedBy)
	query := storekit.SQLf(`UPDATE relationship SET role=coalesce(%s,role), captured_by=%s,
 is_current_primary=coalesce(%s,is_current_primary) AND (kind<>'employment' OR %s),
 started_at=%s, ended_at=%s, employment_status=%s, started_precision=%s, ended_precision=%s
 WHERE id=%s RETURNING %s`, role, captured, primary, employment.IsCurrentSQL(ended, status, endPrecision), started, ended, status, startPrecision, endPrecision, row, relationshipColumns)
	return scanRelationship(tx.QueryRow(ctx, query, args...))
}
