// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// contactComponent is one connected group of open pairs the reader can see.
// The case read, the case list and the case decision all start from it, so
// they agree on members, anchor and size.
type contactComponent struct {
	anchor     ids.UUID
	members    []ids.UUID // id order
	pairs      []componentPair
	confidence float64
}

type componentPair struct {
	left, right ids.UUID
	confidence  float64
}

func (k contactComponent) tooLarge() bool { return len(k.members) > contactCaseMaxMembers }

// readContactComponents reads every component the reader can see, each with
// its lowest member as its anchor.
func readContactComponents(ctx context.Context, tx pgx.Tx) ([]contactComponent, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	edgeRule, err := openContactPairs(ctx, arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT left_contact_id, right_contact_id, confidence FROM dedupe_candidate WHERE `+edgeRule, args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the open contact pairs: %w", err)
	}
	pairs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (componentPair, error) {
		var p componentPair
		return p, row.Scan(&p.left, &p.right, &p.confidence)
	})
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the open contact pairs: %w", err)
	}
	return joinComponents(pairs), nil
}

// joinComponents unions the pairs into components, in anchor order.
func joinComponents(pairs []componentPair) []contactComponent {
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
	for _, p := range pairs {
		a, b := find(p.left), find(p.right)
		if compareUUID(b, a) < 0 {
			a, b = b, a
		}
		root[b] = a
	}
	byRoot := map[ids.UUID]*contactComponent{}
	for id := range root {
		top := find(id)
		if byRoot[top] == nil {
			byRoot[top] = &contactComponent{}
		}
		byRoot[top].members = append(byRoot[top].members, id)
	}
	for _, p := range pairs {
		k := byRoot[find(p.left)]
		k.pairs = append(k.pairs, p)
		k.confidence = max(k.confidence, p.confidence)
	}
	out := make([]contactComponent, 0, len(byRoot))
	for _, k := range byRoot {
		slices.SortFunc(k.members, compareUUID)
		k.anchor = k.members[0]
		out = append(out, *k)
	}
	slices.SortFunc(out, func(a, b contactComponent) int { return compareUUID(a.anchor, b.anchor) })
	return out
}

func componentHolding(components []contactComponent, member ids.UUID) (contactComponent, bool) {
	for _, k := range components {
		if _, found := slices.BinarySearchFunc(k.members, member, compareUUID); found {
			return k, true
		}
	}
	return contactComponent{}, false
}

// shown is what one review of the component holds: at most
// contactCaseMaxMembers members, nearest the addressed one over the pairs
// first and then by id. The addressed member is always among them.
func (k contactComponent) shown(address ids.UUID) []ids.UUID {
	neighbours := map[ids.UUID][]ids.UUID{}
	for _, p := range k.pairs {
		neighbours[p.left] = append(neighbours[p.left], p.right)
		neighbours[p.right] = append(neighbours[p.right], p.left)
	}
	seen := map[ids.UUID]bool{address: true}
	out, ring := []ids.UUID{address}, []ids.UUID{address}
	for len(ring) > 0 && len(out) < contactCaseMaxMembers {
		var next []ids.UUID
		for _, id := range ring {
			for _, n := range neighbours[id] {
				if !seen[n] {
					seen[n] = true
					next = append(next, n)
				}
			}
		}
		slices.SortFunc(next, compareUUID)
		out = append(out, next[:min(len(next), contactCaseMaxMembers-len(out))]...)
		ring = next
	}
	slices.SortFunc(out, compareUUID)
	return out
}
