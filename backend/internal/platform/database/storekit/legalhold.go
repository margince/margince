// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The one spelling of "place or lift a litigation hold".
//
// The flag lives on five tables owned by three modules — contact, company and
// lead in contacts, deal in deals, project in projects — and a module may only
// write its own. So the WRITE is issued by each owner and the SHAPE of it is
// here: read the current value inside the transaction, refuse a no-op, patch
// through the guarded path, and audit with the stated reason.
//
// Shared rather than copied three times because the refusal and the audit
// payload are the whole of the contract's 409 and of the provenance a hold
// depends on. Three copies would drift, and the first thing to drift would be
// the reason — which is the only record of WHY anything is held, no column
// carrying it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// legalHoldColumn is the boolean every one of the five tables carries.
const legalHoldColumn = "legal_hold"

// SetLegalHold places or lifts the hold on one record, auditing the reason.
//
// The caller names its own table as a compile-time literal — nothing here
// takes a name off a request body. Refuses with ErrConflict when the record is
// already in the state asked for: placing twice would write a second audit row
// claiming a change that did not happen, and lifting an unheld record would
// read afterwards as though a hold had been in force.
func SetLegalHold(
	ctx context.Context, tx pgx.Tx, table string, id ids.UUID, held bool, reason string,
) error {
	var current bool
	// The table is a compile-time literal from the owning module and is passed
	// through pgx's own identifier quoting; nothing off a request body reaches it.
	err := tx.QueryRow(ctx,
		`SELECT `+legalHoldColumn+` FROM `+pgx.Identifier{table}.Sanitize()+` WHERE id = $1 AND archived_at IS NULL`,
		id).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read legal hold: %w", err)
	}
	if current == held {
		return apperrors.ErrConflict
	}

	p := NewPatch()
	p.Set(legalHoldColumn, current, held)
	if err := p.ApplyGuarded(ctx, tx, table, id, nil); err != nil {
		return fmt.Errorf("set legal hold: %w", err)
	}
	// The reason rides the audit payload and nowhere else. A column per table
	// would be five more places for it to disagree with the audit row that
	// already carries the actor, the moment and the correlation id.
	after := p.After()
	after["reason"] = reason
	if _, err := Audit(ctx, tx, legalHoldAction(held), table, id, p.Before(), after); err != nil {
		return fmt.Errorf("audit legal hold: %w", err)
	}
	return nil
}

// legalHoldAction names the two directions distinctly, so an audit trail reads
// as what was decided rather than as a column that changed.
func legalHoldAction(held bool) string {
	if held {
		return "place_legal_hold"
	}
	return "lift_legal_hold"
}
