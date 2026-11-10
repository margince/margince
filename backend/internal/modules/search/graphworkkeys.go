// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func interactionPairKeys(pairs []pair) (users, contacts []ids.UUID) {
	users = make([]ids.UUID, 0, len(pairs))
	contacts = make([]ids.UUID, 0, len(pairs))
	for _, p := range pairs {
		users = append(users, p.user)
		contacts = append(contacts, p.contact)
	}
	return users, contacts
}

func contactPairKeys(pairs []contactPair) (first, second []ids.UUID) {
	first = make([]ids.UUID, 0, len(pairs))
	second = make([]ids.UUID, 0, len(pairs))
	for _, p := range pairs {
		first = append(first, p.a)
		second = append(second, p.b)
	}
	return first, second
}

func scanPairs(rows pgx.Rows) ([]pair, error) {
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.user, &p.contact); err != nil {
			return nil, err
		}
		pairs = append(pairs, p)
	}
	return pairs, rows.Err()
}
