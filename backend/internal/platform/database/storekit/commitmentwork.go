// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// CurrentCommitmentTaskClause requires current automatic evidence or adoption by the assignee.
// alias is a compile-time activity alias, never request input.
func CurrentCommitmentTaskClause(alias string, asOf time.Time, arg func(any) int) string {
	pinner := pgx.Identifier{alias, "assignee_id"}.Sanitize()
	alias = pgx.Identifier{alias}.Sanitize()
	source := provenance.CurrentCommitmentClause(alias+".source_activity_id", alias+".source_id", alias+".due_at", asOf, arg)
	return fmt.Sprintf(`(coalesce(%[1]s.source_system, '') <> $%[2]d OR `+HumanWroteActivityClause(alias)+`
 OR (`+pinner+`, %[1]s.id::text) IN (SELECT reader_id, row_id FROM `+auth.EffectiveWorklistPinsSQL(arg)+` adopted WHERE adopted.source='task')
 OR `+source+`)`, alias, arg(provenance.CommitmentTaskSource))
}
