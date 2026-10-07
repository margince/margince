// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// Which direction went last on an account, and when, answered for a SET of
// companies in one statement. The page's last-touch section is this over a set
// of one; the ranked queue asks it for every company a page names, so a row and
// the account it opens cannot disagree about who wrote last.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LastTouch is when the account last wrote to us and when we last wrote to it.
// Nil on either side means it never happened; a withheld answer is a refusal
// from LastTouchFor, never a zero in its map.
type LastTouch struct {
	InboundAt  *time.Time
	OutboundAt *time.Time
}

// LastTouchFor answers the pair that replaced the header's 0-100 score
// (AC-company-2, ADR-0079 arc) for every company in the set the caller may
// read. A company the caller may not read, or one that is archived, is absent;
// a caller who may not read activity is refused outright.
//
// Two timestamps rather than one "last touch", because which side wrote last
// IS the question: an account we mailed a fortnight ago with no reply and one
// that wrote to us this morning have the same last-touch date and opposite
// meanings.
//
// It walks the links the timeline does, so the header can never disagree with
// the list under it, and it carries the caller's activity row scope, so a rep
// sees the last message THEY may read rather than the account's true last one.
// A message dated after now has not been sent yet, so it is neither side's last
// word: a scheduled send must not hide the real last one.
func LastTouchFor(
	ctx context.Context, tx pgx.Tx, companyIDs []ids.CompanyID, now time.Time, opts AssembleOptions,
) (map[ids.CompanyID]LastTouch, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return nil, err
	}
	out := make(map[ids.CompanyID]LastTouch, len(companyIDs))
	if len(companyIDs) == 0 {
		return out, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	wantedPos := arg(companyIDs)
	visible, err := scopeClause(ctx, "company", activities.OuterCompanyAlias, arg)
	if err != nil {
		return nil, err
	}
	scope, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	reached := "FROM activity a WHERE a.archived_at IS NULL AND " + activities.CompanyLinkedActivityExistsPerCompany()
	if scope != "" {
		reached += " AND " + scope
	}
	reached += opts.projectScope(arg)
	reached += fmt.Sprintf(" AND a.occurred_at <= $%d", arg(now))
	// An ordered LIMIT-1 arm per direction rather than a FILTERed max(): an
	// aggregate sees every qualifying row, each arm stops at the newest, so the
	// cost per company is bounded by how far back that message is.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT %[1]s.id,
		       (SELECT a.occurred_at %[2]s AND a.direction = 'inbound' ORDER BY a.occurred_at DESC LIMIT 1),
		       (SELECT a.occurred_at %[2]s AND a.direction = 'outbound' ORDER BY a.occurred_at DESC LIMIT 1)
		FROM company %[1]s
		WHERE %[1]s.id = ANY($%[3]d) AND %[1]s.archived_at IS NULL AND (%[4]s)`,
		activities.OuterCompanyAlias, reached, wantedPos, visible), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var companyID ids.CompanyID
		var touch LastTouch
		if err := rows.Scan(&companyID, &touch.InboundAt, &touch.OutboundAt); err != nil {
			return nil, err
		}
		out[companyID] = touch
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
