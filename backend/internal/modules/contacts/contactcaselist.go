// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The scopes a case list is read in.
const (
	contactCaseScopeMine       = "mine"
	contactCaseScopeUnassigned = "unassigned"
	contactCaseScopeAll        = "all"
)

// rankedContactCases answers every case the reader can see in one scope,
// strongest first. The components are joined over the same edge rule the walk
// uses, so a listed case reads back whole by its anchor.
func rankedContactCases(ctx context.Context, tx pgx.Tx, scope string, recipient ids.UUID) ([]caseSummary, error) {
	keep, err := caseScopeFilter(ctx, scope, recipient)
	if err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	pairs, err := openContactPairs(ctx, arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT left_contact_id, right_contact_id, confidence FROM dedupe_candidate WHERE `+pairs, args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the open contact pairs: %w", err)
	}
	type pair struct {
		left, right ids.UUID
		confidence  float64
	}
	open, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pair, error) {
		var p pair
		return p, row.Scan(&p.left, &p.right, &p.confidence)
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the open contact pairs: %w", err)
	}
	root := map[ids.UUID]ids.UUID{}
	var find func(ids.UUID) ids.UUID
	find = func(id ids.UUID) ids.UUID {
		parent, seen := root[id]
		if !seen || parent == id {
			root[id] = id
			return id
		}
		top := find(parent)
		root[id] = top
		return top
	}
	for _, p := range open {
		a, b := find(p.left), find(p.right)
		if compareUUID(b, a) < 0 {
			a, b = b, a
		}
		root[b] = a
	}
	byRoot := map[ids.UUID]*caseSummary{}
	for _, p := range open {
		top := find(p.left)
		summary, ok := byRoot[top]
		if !ok {
			summary = &caseSummary{}
			byRoot[top] = summary
		}
		summary.confidence = max(summary.confidence, p.confidence)
	}
	for id := range root {
		summary := byRoot[find(id)]
		summary.members = append(summary.members, id)
	}
	return keepRankedCases(ctx, tx, byRoot, keep)
}

// keepRankedCases settles each component's anchor, keeps those in scope and
// ranks them.
func keepRankedCases(ctx context.Context, tx pgx.Tx, byRoot map[ids.UUID]*caseSummary, keep func([]caseOwnership) bool) ([]caseSummary, error) {
	var all []ids.UUID
	for _, summary := range byRoot {
		slices.SortFunc(summary.members, compareUUID)
		summary.anchor = summary.members[0]
		all = append(all, summary.members...)
	}
	owners, err := readCaseOwnership(ctx, tx, all)
	if err != nil {
		return nil, err
	}
	var out []caseSummary
	for _, summary := range byRoot {
		held := make([]caseOwnership, 0, len(summary.members))
		for _, id := range summary.members {
			held = append(held, owners[id])
		}
		if keep(held) {
			out = append(out, *summary)
		}
	}
	slices.SortFunc(out, func(a, b caseSummary) int {
		if a.confidence != b.confidence {
			if a.confidence > b.confidence {
				return -1
			}
			return 1
		}
		return compareUUID(a.anchor, b.anchor)
	})
	return out, nil
}

// caseOwnership is who owns one member, and whether that owner is a live seat.
type caseOwnership struct {
	owner *ids.UUID
	live  bool
}

// caseScopeFilter answers whether a case belongs to the requested scope.
// Mine holds a member the recipient owns; the recipient is the caller when
// none is named. Unassigned holds no live owner.
func caseScopeFilter(ctx context.Context, scope string, recipient ids.UUID) (func([]caseOwnership) bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil, fmt.Errorf("contacts: no actor bound to read cases for")
	}
	if recipient.IsZero() {
		recipient = actor.UserID
	}
	switch scope {
	case "", contactCaseScopeMine:
		return func(held []caseOwnership) bool {
			return slices.ContainsFunc(held, func(o caseOwnership) bool { return o.owner != nil && *o.owner == recipient })
		}, nil
	case contactCaseScopeUnassigned:
		return func(held []caseOwnership) bool {
			return !slices.ContainsFunc(held, func(o caseOwnership) bool { return o.live })
		}, nil
	case contactCaseScopeAll:
		return func([]caseOwnership) bool { return true }, nil
	}
	return nil, &DedupeInputError{Field: "scope", Msg: "must be mine, unassigned or all"}
}

func readCaseOwnership(ctx context.Context, tx pgx.Tx, memberIDs []ids.UUID) (map[ids.UUID]caseOwnership, error) {
	out := map[ids.UUID]caseOwnership{}
	if len(memberIDs) == 0 {
		return out, nil
	}
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT contact.id, contact.owner_id, `+liveCaseOwnerSQL+`
		  FROM contact LEFT JOIN app_user u ON u.id = contact.owner_id
		 WHERE contact.id = ANY(@members)`, pgx.NamedArgs{"members": memberIDs})
	if err != nil {
		return nil, fmt.Errorf("contacts: reading who owns the case members: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var held caseOwnership
		if err := rows.Scan(&id, &held.owner, &held.live); err != nil {
			return nil, err
		}
		out[id] = held
	}
	return out, rows.Err()
}
