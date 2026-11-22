// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What the personal-mail sweep keeps, counted apart from what it destroys.

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// KeptCounts is how many messages a purge left standing, by the reason.
type KeptCounts struct {
	Held                   int
	UnderStatute           int
	UnderRequest           int
	UnderUndeterminedFloor int
}

// KeptCounts is the subject's withheld messages, counted by reason.
func (s PurgeSubject) KeptCounts() KeptCounts {
	return KeptCounts{
		Held:                   len(s.Held),
		UnderStatute:           len(s.UnderStatute),
		UnderRequest:           len(s.UnderRequest),
		UnderUndeterminedFloor: len(s.UnderUndeterminedFloor),
	}
}

// personalKeptByShield is the condition under which a due personal message is
// kept rather than destroyed: the statutory shield or an open request.
// SelectPersonalPurgeTx negates it and CountPersonalKeptTx asserts it, so the
// sweep destroys what the receipt does not list as kept.
func personalKeptByShield(shielded string) string {
	return `((` + shielded + `) OR ` + underAnOpenRequest + `)`
}

// CountPersonalKeptTx counts every due personal message of one seat that a
// shield keeps, by reason, whatever one sweep batch happened to reach.
//
// Held (`restricted_at`) is always zero. PersonalPurgeScope leaves a restricted
// message out of what is due, so the sweep never owed its deletion.
func CountPersonalKeptTx(
	ctx context.Context, tx pgx.Tx, user ids.UUID, windows PersonalPurgeWindows, floor StatutoryFloor,
) (KeptCounts, error) {
	var kept KeptCounts
	if user == ids.Nil {
		return kept, nil
	}
	args := []any{windows.ByOwner, windows.ByClassifier, user}
	shielded, args := floor.column(len(args), args)
	rows, err := tx.Query(ctx, `
		SELECT `+withheldReason(shielded, floor.shieldedAs())+` AS withheld, count(*)
		  FROM activity a
		  JOIN capture_import i ON i.activity_id = a.id AND i.user_id = $3
		  JOIN capture_pending_counterparty p
		    ON p.email = a.counterparty_email AND p.owner_id = $3
		 WHERE `+personalPurgeDue("$3")+`
		   AND `+personalKeptByShield(shielded)+`
		 GROUP BY 1`, args...)
	if err != nil {
		return kept, fmt.Errorf("capture: counting personal mail a shield keeps: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var reason string
		var n int
		if err := rows.Scan(&reason, &n); err != nil {
			return kept, fmt.Errorf("capture: counting personal mail a shield keeps: %w", err)
		}
		switch reason {
		case withheldByStatute:
			kept.UnderStatute = n
		case withheldByRequest:
			kept.UnderRequest = n
		case withheldByUndeterminedFloor:
			kept.UnderUndeterminedFloor = n
		}
	}
	if err := rows.Err(); err != nil {
		return kept, fmt.Errorf("capture: counting personal mail a shield keeps: %w", err)
	}
	return kept, nil
}
