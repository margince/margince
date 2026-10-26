// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// A legal hold on a row of a table an extension owns. The table carries no
// `legal_hold` column, so the hold is a row of `extension_legal_hold`, which
// this module owns, and the owner of the row is not involved. The place and
// lift audit rows are the ones a core record's hold writes, so the trail reads
// the same wherever the hold sits.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// setExtensionHold places or lifts the hold on one row of a declared table.
//
// Gated like the retention policies it overrides: a hold decides what a sweep
// may destroy, which is the controller's judgement and not the unit's.
func (h Handlers) setExtensionHold(ctx context.Context, table string, id ids.UUID, held bool, reason string) error {
	if err := auth.Require(ctx, retentionPolicyObject, principal.ActionUpdate); err != nil {
		return err
	}
	declared, ok := h.eraser.extensions.Table(table)
	if !ok {
		return fmt.Errorf("legal hold on %q, a table no extension declared: %w", table, apperrors.ErrInvalidArgument)
	}
	return h.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := h.eraser.extensions.SetHeld(ctx, tx, declared, id, held); err != nil {
			return err
		}
		return storekit.AuditLegalHold(ctx, tx, table, id, held, reason)
	})
}
