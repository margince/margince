// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PurgeRemovedAdmitted is PurgeRemoved with admit asked first in the
// transaction that selects what the deletion destroys.
func (p *CapturePurger) PurgeRemovedAdmitted(
	ctx context.Context, admit func(context.Context, pgx.Tx) error, seat ids.UUID, sourceSystem, sourceID string,
) error {
	var subject capture.PurgeSubject
	if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		if admit != nil {
			if err := admit(ctx, tx); err != nil {
				return err
			}
		}
		floor, _, err := purgeFloorTx(ctx, tx)
		if err != nil {
			return err
		}
		subject, err = capture.SelectRemovedPurgeTx(ctx, tx, seat, sourceSystem, sourceID, floor)
		return err
	}); err != nil {
		return fmt.Errorf("capture: selecting what a mailbox-side deletion destroys: %w", err)
	}
	if subject.Total() == 0 {
		// Nothing captured under that key, or not by this seat. Most deleted
		// mail was never captured, so this is not an error.
		return nil
	}
	if len(subject.SoleImports) == 0 {
		slog.InfoContext(ctx, "capture: a mailbox-side deletion left the message standing",
			"source_system", sourceSystem,
			"shared_with_colleagues", len(subject.SharedImports),
			"withheld", len(subject.Restricted))
		return nil
	}
	// No contacts are anonymised. A deletion speaks about one mail, and the
	// owner's exclusion rule is where a correspondent is judged.
	if _, err := p.carryOut(ctx, subject, nil, nil, seat, privacy.PurgeMailboxDeletion); err != nil {
		return fmt.Errorf("capture: destroying a message the owner deleted at the provider: %w", err)
	}
	return nil
}
