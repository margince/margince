// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// eitherSideMaskedTx answers whether masking changed the arriving copy, or the
// stored row holds the mask. Masking makes two different secrets read the same,
// so equal text then proves nothing about which message a seat holds. Text a
// sender typed as "[masked]" with nothing masked keeps the equality proof,
// unless the stored row holds the mark too.
func eitherSideMaskedTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, fields ActivityFields) (bool, error) {
	if fields.CredentialsMasked {
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

// maskedReplayIsProvenTx is the replay proof for masked mail, in place of equal
// text. The stored row must name, as a To or Cc recipient, an address the
// provider attested as this seat's mailbox and no other seat's
// (ProvedUnambiguouslyTx). The first capture wrote those recipients from its own
// copy, so a forger who put themselves on Cc of a copy is not among them. A
// self-declared alias is no proof, and neither is being the sender. The stored
// files must be ones this capture carried, as on the text path.
//
// A colleague only on Bcc, or on IMAP, keeps a row of their own, which is the
// safe direction.
func maskedReplayIsProvenTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, seat ids.UUID, carried []string) (bool, error) {
	var filesCarried bool
	args := []any{id, carried}
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $1 AND `+storedFilesWereCarried(len(args))+`)`,
		args...).Scan(&filesCarried); err != nil {
		return false, fmt.Errorf("capture: comparing the files of %s: %w", id, err)
	}
	if !filesCarried {
		return false, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT address FROM activity_participant
		 WHERE activity_id = $1 AND address IS NOT NULL AND role IN ('to', 'cc')`, id)
	if err != nil {
		return false, fmt.Errorf("capture: reading the recipients of %s: %w", id, err)
	}
	recipients, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return false, fmt.Errorf("capture: reading the recipients of %s: %w", id, err)
	}
	for _, address := range recipients {
		proved, err := ProvedUnambiguouslyTx(ctx, tx, seat, address)
		if err != nil || proved {
			return proved, err
		}
	}
	return false, nil
}

// storedFilesWereCarried is the condition that every file stored on activity a
// has a checksum among the ones in the placeholder $n. A file kept by name only
// has no checksum, so a row carrying one never passes.
func storedFilesWereCarried(n int) string {
	return fmt.Sprintf(`NOT EXISTS (
		SELECT 1 FROM attachment f
		 WHERE f.activity_id = a.id
		   AND (f.checksum IS NULL OR f.checksum <> ALL($%d)))`, n)
}
