// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// eitherSideMaskedTx answers whether the arriving or the stored subject or body
// holds the credential mask. Masking makes two different secrets read the same,
// so equal masked text proves nothing about which message a seat holds.
func eitherSideMaskedTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, fields ActivityFields) (bool, error) {
	if strings.Contains(fields.Subject, credentialmask.Marker) || strings.Contains(fields.Body, credentialmask.Marker) {
		return true, nil
	}
	var stored bool
	if err := tx.QueryRow(ctx, `
		SELECT strpos(coalesce(subject, ''), $2) > 0 OR strpos(coalesce(body, ''), $2) > 0
		  FROM activity WHERE id = $1`,
		id, credentialmask.Marker).Scan(&stored); err != nil {
		return false, fmt.Errorf("capture: reading whether %s holds masked text: %w", id, err)
	}
	return stored, nil
}

// storedRowNamesSeatTx is the replay proof for masked mail. The stored row must
// name one of the seat's own exact addresses as a party. The first capture wrote
// those parties from its own copy. A seat that copied the Message-ID into a mail
// to itself is not among them. A colleague only on Bcc is not named either
// and keeps a row of their own, which is the safe direction.
func storedRowNamesSeatTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (bool, error) {
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil || self.Empty() {
		return false, err
	}
	rows, err := tx.Query(ctx, `
		SELECT address FROM activity_participant
		 WHERE activity_id = $1 AND address IS NOT NULL`, id)
	if err != nil {
		return false, fmt.Errorf("capture: reading the parties of %s: %w", id, err)
	}
	addresses, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return false, fmt.Errorf("capture: reading the parties of %s: %w", id, err)
	}
	return slices.ContainsFunc(addresses, self.CoversAddressExactly), nil
}
