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
	object   string
	table    string
	own      func(alias string, me int, arg func(any) int) string
	writable func(ctx context.Context, tx pgx.Tx, p principal.Principal, rowIDs []ids.UUID) (map[ids.UUID]bool, error)
}

// liveDealWrites is the row write gate the deal archive asks, over a page.
func liveDealWrites(ctx context.Context, tx pgx.Tx, p principal.Principal, rowIDs []ids.UUID) (map[ids.UUID]bool, error) {
	writable, err := rowWriteSubset(ctx, tx, p, tableDeal, rowIDs)
	if err != nil {
		return nil, err
	}
	archived, err := archivedAmong(ctx, tx, tableDeal, writable)
	if err != nil {
		return nil, err
	}
	for id := range archived {
		delete(writable, id)
	}
	return writable, nil
}

func activityWrites(ctx context.Context, tx pgx.Tx, _ principal.Principal, rowIDs []ids.UUID) (map[ids.UUID]bool, error) {
	return ActivityWritableSubset(ctx, tx, rowIDs)
}

// ActivityArchive admits the author of a hand-logged note or task. Captured
// correspondence and a colleague's notes keep needing activity.delete.
var ActivityArchive = ArchiveRule{object: "activity", table: "activity", writable: activityWrites, own: func(alias string, me int, arg func(any) int) string {
	return fmt.Sprintf(`(%[1]s.kind IN ('note', 'task') AND %[1]s.source = $%[2]d AND %[3]s)`,
		alias, arg(provenance.RecordSourceManual), activityAuthorSQL(alias, me, arg))
}}

// DealArchive admits the deal's owner, unless the owner took the deal
// themselves. A write share lets its holder name themselves owner, so without
// that arm the share would carry the archive the decision keeps from it.
var DealArchive = ArchiveRule{object: tableDeal, table: tableDeal, writable: liveDealWrites, own: func(alias string, me int, _ func(any) int) string {
	return fmt.Sprintf(`(%[1]s.owner_id IN (%[2]s) AND NOT EXISTS (
	   SELECT 1 FROM (SELECT made.action, made.actor_id, made.on_behalf_of FROM audit_log made
	     WHERE made.entity_type = 'deal' AND made.entity_id = %[1]s.id
	       AND made.before->'owner_id' IS DISTINCT FROM made.after->'owner_id' AND made.after ? 'owner_id'
	     ORDER BY made.occurred_at DESC, made.id DESC LIMIT 1) handed
	   WHERE handed.action <> 'create' AND (handed.on_behalf_of IN (%[2]s)
	      OR (split_part(handed.actor_id, ':', 1) = '%[3]s'
	          AND split_part(handed.actor_id, ':', 2) IN (SELECT taker.id::text FROM (%[2]s) taker)))))`,
		alias, memberIdentitySQL(me), principal.PrincipalHuman)
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

// ArchivableSubset answers, for a page, which rows the caller may archive:
// the seat, the row write gate EnsureArchivable's callers ask, and the rule.
// It is the whole answer, so a delete holder without update is not refused.
func ArchivableSubset(ctx context.Context, tx pgx.Tx, r ArchiveRule, rowIDs []ids.UUID) (map[ids.UUID]bool, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return nil, err
	}
	canDelete := Allows(ctx, r.object, principal.ActionDelete)
	mutable := p.Type == principal.PrincipalSystem || p.SeatType.CanMutate()
	if !mutable || len(rowIDs) == 0 || (!canDelete && !Allows(ctx, r.object, principal.ActionUpdate)) {
		return map[ids.UUID]bool{}, nil
	}
	writable, err := r.writable(ctx, tx, p, rowIDs)
	if err != nil || canDelete {
		return writable, err
	}
	candidates := make([]ids.UUID, 0, len(writable))
	for id, may := range writable {
		if may {
			candidates = append(candidates, id)
		}
	}
	if len(candidates) == 0 {
		return map[ids.UUID]bool{}, nil
	}
	return ownedArchivable(ctx, tx, r, candidates)
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
