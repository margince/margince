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
	status := relationshipStatusPatch("patched.employment_status", in, arg)
	precision := relationshipPrecisionPatch("patched.ended_precision", in.EndedPrecision, in.ClearEndedAt || in.ClearEndedPrecision, arg)
	return flipCurrentPrimary(ctx, tx, storekit.SQLf(`UPDATE relationship SET is_current_primary=false
 WHERE contact_id=%s AND id<>%s AND %s
 AND EXISTS(SELECT 1 FROM relationship patched WHERE patched.id=%s AND %s)`,
		contact, row, employment.CurrentPrimarySlotSQL(""), row, employment.IsCurrentSQL(ended, status, precision)), args...)
}

func patchRelationshipRow(ctx context.Context, tx pgx.Tx, id ids.UUID, in UpdateRelationshipInput, capturedBy string) (relationshipRow, error) {
	var args []any
	arg := func(v any) string { args = append(args, v); return storekit.SQLf("$%d", len(args)) }
	row, primary := arg(id), arg(in.IsCurrentPrimary)
	role := storekit.SQLf("CASE WHEN %s::boolean THEN NULL ELSE coalesce(%s::text, role) END", arg(in.ClearRole), arg(in.Role))
	started := storekit.SQLf("CASE WHEN %s::boolean THEN NULL ELSE coalesce(%s::date, started_at) END", arg(in.ClearStartedAt), arg(in.StartedAt))
	ended := storekit.SQLf("CASE WHEN %s::boolean THEN NULL ELSE coalesce(%s::date, ended_at) END", arg(in.ClearEndedAt), arg(in.EndedAt))
	status := relationshipStatusPatch("employment_status", in, arg)
	startPrecision := relationshipPrecisionPatch("started_precision", in.StartedPrecision, in.ClearStartedAt || in.ClearStartedPrecision, arg)
	endPrecision := relationshipPrecisionPatch("ended_precision", in.EndedPrecision, in.ClearEndedAt || in.ClearEndedPrecision, arg)
	captured := arg(capturedBy)
	query := storekit.SQLf(`UPDATE relationship SET role=%s, captured_by=%s,
 is_current_primary=coalesce(%s,is_current_primary) AND (kind<>'employment' OR %s),
 started_at=%s, ended_at=%s, employment_status=%s, started_precision=%s, ended_precision=%s
 WHERE id=%s RETURNING %s`, role, captured, primary, employment.IsCurrentSQL(ended, status, endPrecision), started, ended, status, startPrecision, endPrecision, row, relationshipColumns)
	return scanRelationship(tx.QueryRow(ctx, query, args...))
}

func relationshipStatusPatch(column string, in UpdateRelationshipInput, arg func(any) string) string {
	return storekit.SQLf("CASE WHEN %s::boolean THEN NULL ELSE coalesce(%s::text, %s) END", arg(in.ClearEmploymentStatus), arg(in.EmploymentStatus), column)
}

func relationshipPrecisionPatch(column string, value *string, clearValue bool, arg func(any) string) string {
	return storekit.SQLf("CASE WHEN %s::boolean THEN NULL ELSE coalesce(%s::text, %s) END", arg(clearValue), arg(value), column)
}
