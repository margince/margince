// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func tombstoneLeadsAndSignals(ctx context.Context, tx pgx.Tx, subject ids.ContactID, leads []ids.UUID, reason string) error {
	if err := tombstoneCollateralScrubs(ctx, tx, "lead", leads, reason, causeContactErasure); err != nil {
		return err
	}
	return tombstoneSubjectSignals(ctx, tx, subject, reason)
}

// tombstoneSubjectSignals cuts the audit history of every signal about the
// subject. A corrected signal keeps its old kind and evidence in the before
// image of an update row, and evidence quotes the source. The audit spine is
// append-only, so the history reads withhold those images by this tombstone.
func tombstoneSubjectSignals(ctx context.Context, tx pgx.Tx, subject ids.ContactID, reason string) error {
	rows, err := tx.Query(ctx,
		`SELECT id FROM signal
		  WHERE (entity_type = 'contact' AND entity_id = $1) OR resolved_contact_id = $1`, subject)
	if err != nil {
		return fmt.Errorf("privacy: reading the subject's signals: %w", err)
	}
	signals, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("privacy: collecting the subject's signals: %w", err)
	}
	return tombstoneCollateralScrubs(ctx, tx, "signal", signals, reason, causeContactErasure)
}
