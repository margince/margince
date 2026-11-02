// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WriteRefusal names why a caller may not change a row, so a client can say
// why and point at the fix instead of hiding the edit without a word.
type WriteRefusal string

// The refusals, most general first: a read seat outranks a role, which
// outranks anything about the row itself.
const (
	RefusalSeatReadOnly WriteRefusal = "seat_read_only"
	RefusalRole         WriteRefusal = "role"
	RefusalArchived     WriteRefusal = "archived"
	RefusalUnowned      WriteRefusal = "unowned"
	RefusalNotYours     WriteRefusal = "not_yours"
)

// RowWrite is what a client is told about one row: whether the caller may
// change it, and the refusal when they may not.
type RowWrite struct {
	May     bool
	Refusal WriteRefusal
}

// Contract is the wire's `write_refusal`: absent when the caller may write.
func (w RowWrite) Contract() *crmcontracts.WriteRefusal {
	if w.Refusal == "" {
		return nil
	}
	refusal := crmcontracts.WriteRefusal(w.Refusal)
	return &refusal
}

// callerRefusal is the refusal that holds for every row of the table at once,
// read from the principal alone. The seat is compared against SeatRead for the
// reason writeAuthorityPredicateAs gives: an unset seat is an internal caller.
func callerRefusal(p principal.Principal, table string) WriteRefusal {
	if p.SeatType == principal.SeatRead {
		return RefusalSeatReadOnly
	}
	if p.Type != principal.PrincipalSystem && !p.Permissions.Allows(table, principal.ActionUpdate) {
		return RefusalRole
	}
	return ""
}

// rowRefusals answers each row: archived for a row the caller could otherwise
// change, unowned for an ownerless one (it is claimed before it is written),
// and not_yours for the rest. Each of the two probes runs only when a row
// needs it, so a page the caller owns whole costs the one archive probe.
func rowRefusals(ctx context.Context, tx pgx.Tx, table string, rowIDs []ids.UUID,
	writable map[ids.UUID]bool,
) (map[ids.UUID]RowWrite, error) {
	archived, err := archivedAmong(ctx, tx, table, writable)
	if err != nil {
		return nil, err
	}
	var refused []ids.UUID
	for _, id := range rowIDs {
		if !writable[id] {
			refused = append(refused, id)
		}
	}
	unowned, err := unownedAmong(ctx, tx, table, refused)
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]RowWrite, len(rowIDs))
	for _, id := range rowIDs {
		switch {
		case writable[id] && archived[id]:
			out[id] = RowWrite{Refusal: RefusalArchived}
		case writable[id]:
			out[id] = RowWrite{May: true}
		case unowned[id]:
			out[id] = RowWrite{Refusal: RefusalUnowned}
		default:
			out[id] = RowWrite{Refusal: RefusalNotYours}
		}
	}
	return out, nil
}

// unownedAmong is the subset of rows with no owner. The table name is one of
// shareableTables, checked by WritableSubset before this runs.
func unownedAmong(ctx context.Context, tx pgx.Tx, table string, rowIDs []ids.UUID) (map[ids.UUID]bool, error) {
	unowned := map[ids.UUID]bool{}
	if len(rowIDs) == 0 {
		return unowned, nil
	}
	rows, err := tx.Query(ctx,
		fmt.Sprintf(`SELECT id FROM %s WHERE id = ANY($1) AND owner_id IS NULL`,
			pgx.Identifier{table}.Sanitize()), rowIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		unowned[id] = true
	}
	return unowned, rows.Err()
}
