// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// How a decision made after capture, a verdict or a human's accept, files mail
// only a shared mailbox captured. It asks what the capture asked: the reader
// team's view, and the backup owner's standing and rules as of this moment.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SharedMailboxFiling is what a later decision may do with shared mail.
type SharedMailboxFiling struct {
	// Shared says only a shared mailbox captured the mail for this question.
	Shared     bool
	ReaderTeam ids.UUID
	// Owner is the backup owner who may still hold a new record, or zero.
	Owner ids.UUID
}

// OwnerFor is who a record created for this question belongs to: owner for
// a seat's own mail, the backup owner or nobody for shared mail.
func (f SharedMailboxFiling) OwnerFor(owner ids.UUID) ids.UUID {
	if !f.Shared {
		return owner
	}
	return f.Owner
}

// SharedMailboxOnlyTx answers the shared mailbox that alone captured activity
// for the question owner owns, and its reader team. A seat whose own mailbox
// also holds the mail asks as on any personal question.
func SharedMailboxOnlyTx(ctx context.Context, tx pgx.Tx, activity, owner ids.UUID) (team ids.UUID, backup *ids.UUID, shared bool, err error) {
	err = tx.QueryRow(ctx, `SELECT c.reader_team_id, c.backup_owner_id FROM capture_import ci
	  JOIN capture_connection c ON c.id = ci.connection_id AND c.user_id IS NULL
	 WHERE ci.activity_id = $1
	   AND NOT EXISTS (SELECT 1 FROM capture_import own WHERE own.activity_id = $1 AND own.user_id = $2)
	 ORDER BY ci.connection_id LIMIT 1`, activity, owner).Scan(&team, &backup)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil, false, nil
	}
	if err != nil {
		return ids.Nil, nil, false, fmt.Errorf("capture: reading whether a shared mailbox alone captured the mail: %w", err)
	}
	return team, backup, true, nil
}

// SharedMailboxFilingTx answers how a decision about owner's question on
// activity files shared mail. A record is minted only while owner is still
// the mailbox's backup owner, may hold its work, and keeps no rule against it.
func SharedMailboxFilingTx(ctx context.Context, tx pgx.Tx, authority HeldAuthority, activity, owner ids.UUID) (SharedMailboxFiling, error) {
	team, backup, shared, err := SharedMailboxOnlyTx(ctx, tx, activity, owner)
	if err != nil || !shared {
		return SharedMailboxFiling{}, err
	}
	filing := SharedMailboxFiling{Shared: true, ReaderTeam: team}
	if backup == nil || *backup != owner || owner.IsZero() {
		return filing, nil
	}
	if _, holds, err := backupMayHoldTx(ctx, tx, authority, team, owner); err != nil || !holds {
		return filing, err
	}
	// The exclusions judge the mail's addresses, read off its participants.
	rows, err := tx.Query(ctx, `SELECT DISTINCT address FROM activity_participant
	  WHERE activity_id = $1 AND address IS NOT NULL ORDER BY address`, activity)
	if err != nil {
		return filing, fmt.Errorf("capture: reading the shared mail a decision files: %w", err)
	}
	addresses, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return filing, fmt.Errorf("capture: reading the shared mail a decision files: %w", err)
	}
	keptOut, err := backupKeepsOutTx(ctx, tx, connector.NormalizedRecord{Addresses: addresses}, activity, owner)
	if err != nil || keptOut {
		return filing, err
	}
	filing.Owner = owner
	return filing, nil
}
