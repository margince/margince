// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MailRecipientEvidence carries parsed headers from a stored original to its owner writer.
type MailRecipientEvidence struct {
	Activity ids.ActivityID
	Envelope *MailEnvelope
}

// RestoreMailRecipientsTx restores address evidence only, never user-based audience grants.
func RestoreMailRecipientsTx(ctx context.Context, tx pgx.Tx, evidence []MailRecipientEvidence) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	identities, err := mailSeatIdentities(ctx, tx)
	if err != nil {
		return 0, err
	}
	addresses := map[string]bool{}
	for _, values := range identities {
		for _, address := range values {
			addresses[address] = true
		}
	}
	changed := 0
	for _, item := range evidence {
		var before int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM activity_participant WHERE activity_id = @id", pgx.NamedArgs{"id": item.Activity}).Scan(&before); err != nil {
			return 0, err
		}
		if err := stampMailRecipients(ctx, tx, item.Activity, item.Envelope, func(address string) bool { return addresses[address] }); err != nil {
			return 0, err
		}
		var after int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM activity_participant WHERE activity_id = @id", pgx.NamedArgs{"id": item.Activity}).Scan(&after); err != nil {
			return 0, err
		}
		if after > before {
			changed++
		}
	}
	return changed, nil
}
