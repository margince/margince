// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ScanUUIDColumn drains an already-executed single-column id query into a
// slice, closing rows itself.
//
// A caller whose query is not the fixed predicate shape SelectIDs runs — a
// hand-written join, say — still faces the same scan loop, and that loop was
// showing up verbatim at each such call site rather than once.
func ScanUUIDColumn(rows pgx.Rows, verb string) ([]ids.UUID, error) {
	defer rows.Close()
	var out []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("%s: %w", verb, err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", verb, err)
	}
	return out, nil
}
