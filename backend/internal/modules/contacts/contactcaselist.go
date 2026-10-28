// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
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
// strongest first, each addressed from the member its reader is shown it by.
func rankedContactCases(ctx context.Context, tx pgx.Tx, scope string, recipient ids.UUID) ([]caseSummary, error) {
	keep, err := caseScopeFilter(ctx, scope, recipient)
	if err != nil {
		return nil, err
	}
	components, err := readContactComponents(ctx, tx)
	if err != nil {
		return nil, err
	}
	var all []ids.UUID
	for _, k := range components {
		all = append(all, k.members...)
	}
	owners, err := readCaseOwnership(ctx, tx, all)
	if err != nil {
		return nil, err
	}
	var out []caseSummary
	for _, k := range components {
		held := make([]caseOwnership, 0, len(k.members))
		for _, id := range k.members {
			held = append(held, owners[id])
		}
		if at, ok := keep(held); ok {
			out = append(out, caseSummary{component: k, address: k.members[at]})
		}
	}
	slices.SortFunc(out, func(a, b caseSummary) int {
		if a.component.confidence != b.component.confidence {
			if a.component.confidence > b.component.confidence {
				return -1
			}
			return 1
		}
		return compareUUID(a.component.anchor, b.component.anchor)
	})
	return out, nil
}

// caseOwnership is who owns one member, and whether that owner is a live seat.
type caseOwnership struct {
	owner *ids.UUID
	live  bool
}

// caseScopeFilter answers whether a case belongs to the requested scope, and
// which member addresses it there. Mine holds a member the recipient owns,
// and addresses it from the first one; the recipient is the caller when none
// is named. Unassigned holds no live owner.
func caseScopeFilter(ctx context.Context, scope string, recipient ids.UUID) (func([]caseOwnership) (int, bool), error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil, errors.New("contacts: no actor bound to read cases for")
	}
	if recipient.IsZero() {
		recipient = actor.UserID
	}
	switch scope {
	case "", contactCaseScopeMine:
		return func(held []caseOwnership) (int, bool) {
			at := slices.IndexFunc(held, func(o caseOwnership) bool { return o.owner != nil && *o.owner == recipient })
			return at, at >= 0
		}, nil
	case contactCaseScopeUnassigned:
		return func(held []caseOwnership) (int, bool) {
			return 0, !slices.ContainsFunc(held, func(o caseOwnership) bool { return o.live })
		}, nil
	case contactCaseScopeAll:
		return func([]caseOwnership) (int, bool) { return 0, true }, nil
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
