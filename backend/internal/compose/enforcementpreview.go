// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// EnforcementPreview counts what switching enforcement on would do, through
// the treatment every forecast reading applies to a deal. It reads in the
// caller's transaction, so a switch can save against the counts it read.
func EnforcementPreview(ctx context.Context, tx pgx.Tx) (assurance.EnforcementPreview, error) {
	var out assurance.EnforcementPreview
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return out, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := fmt.Sprintf(`
		WITH treated AS (
		    SELECT d.id, %[1]s AS treatment FROM deal d
		     WHERE d.status = 'open' AND d.archived_at IS NULL)
		SELECT count(*) FILTER (WHERE treatment IN ($%[2]d, $%[3]d)),
		       count(*) FILTER (WHERE treatment = $%[4]d),
		       md5($%[5]d || '|' || coalesce(string_agg(id::text || ':' || treatment, ',' ORDER BY id), ''))
		  FROM treated`,
		viabilityQualificationSQL("d", arg),
		arg(string(viability.QualificationExcluded)), arg(string(viability.QualificationUnsupported)),
		arg(string(viability.QualificationUnknown)), arg(viability.PolicyVersion))
	if err := tx.QueryRow(ctx, query, args...).Scan(&out.LeftOut, &out.Unknown, &out.Digest); err != nil {
		return out, fmt.Errorf("compose: previewing enforcement: %w", err)
	}
	return out, nil
}
