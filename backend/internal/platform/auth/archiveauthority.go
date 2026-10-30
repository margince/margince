// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// ArchiveRule names who may archive a row without the object's delete grant.
// It is a holder of update for whom own matches the row.
type ArchiveRule struct {
	object string
	table  string
	own    func(alias string, me int, arg func(any) int) string
}

// ActivityArchive admits the author of a hand-logged note or task. Captured
// correspondence and a colleague's notes keep needing activity.delete.
var ActivityArchive = ArchiveRule{object: "activity", table: "activity", own: func(alias string, me int, arg func(any) int) string {
	return fmt.Sprintf(`(%[1]s.kind IN ('note', 'task') AND %[1]s.source = $%[2]d AND %[3]s)`,
		alias, arg(provenance.RecordSourceManual), activityAuthorSQL(alias, me, arg))
}}

// DealArchive admits the deal's owner.
var DealArchive = ArchiveRule{object: tableDeal, table: tableDeal, own: func(alias string, me int, _ func(any) int) string {
	return fmt.Sprintf(`%s.owner_id IN (%s)`, alias, memberIdentitySQL(me))
}}

// RequireArchiveGrant is the up-front half of EnsureArchivable: it refuses a
// caller who could archive no row of the table at all, before a transaction.
func RequireArchiveGrant(ctx context.Context, r ArchiveRule) error {
	return RequireAny(ctx, r.object, principal.ActionDelete, principal.ActionUpdate)
}

// EnsureArchivable admits the archive of one row under r. It answers the
// object grant and the row's own arm only. The caller still asks the row write
// gate, which keeps a colleague's record out of reach.
func EnsureArchivable(ctx context.Context, tx pgx.Tx, r ArchiveRule, id ids.UUID) error {
	refusal := Require(ctx, r.object, principal.ActionDelete)
	if refusal == nil || !Allows(ctx, r.object, principal.ActionUpdate) {
		return refusal
	}
	owned, err := ownedArchivable(ctx, tx, r, []ids.UUID{id})
	if err != nil {
		return err
	}
	if !owned[id] {
		return refusal
	}
	return nil
}

// ArchivableSubset answers EnsureArchivable for a page at once. The caller
// ANDs it with the row's writable flag, as the archive itself asks both.
func ArchivableSubset(ctx context.Context, tx pgx.Tx, r ArchiveRule, rowIDs []ids.UUID) (map[ids.UUID]bool, error) {
	if Allows(ctx, r.object, principal.ActionDelete) {
		all := make(map[ids.UUID]bool, len(rowIDs))
		for _, id := range rowIDs {
			all[id] = true
		}
		return all, nil
	}
	if !Allows(ctx, r.object, principal.ActionUpdate) || len(rowIDs) == 0 {
		return map[ids.UUID]bool{}, nil
	}
	return ownedArchivable(ctx, tx, r, rowIDs)
}

func ownedArchivable(ctx context.Context, tx pgx.Tx, r ArchiveRule, rowIDs []ids.UUID) (map[ids.UUID]bool, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]bool, len(rowIDs))
	if p.UserID.IsZero() {
		return out, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(rowIDs)
	own := r.own("x", arg(p.UserID), arg)
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT x.id FROM %s x WHERE x.id = ANY($%d) AND %s`,
		pgx.Identifier{r.table}.Sanitize(), idsPos, own), args...)
	if err != nil {
		return nil, fmt.Errorf("auth: reading which %s rows the caller may archive: %w", r.table, err)
	}
	owned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("auth: reading which %s rows the caller may archive: %w", r.table, err)
	}
	for _, id := range owned {
		out[id] = true
	}
	return out, nil
}
