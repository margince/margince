// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LockInquiryLeadTx holds the lead a website inquiry names before its task, preserving
// assignment lock order. Visibility is asked under the lock, so an archive that lands first refuses the hold.
func (s *Store) LockInquiryLeadTx(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return err
	}
	if _, err := storekit.LockRow(ctx, tx, "lead", id, storekit.LiveOnly); err != nil {
		return err
	}
	return auth.EnsureVisibleLive(ctx, tx, "lead", id)
}

// LockInquiryContactTx is LockInquiryLeadTx for an inquiry that names a contact.
func (s *Store) LockInquiryContactTx(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return err
	}
	if _, err := storekit.LockRow(ctx, tx, "contact", id, storekit.LiveOnly); err != nil {
		return err
	}
	return auth.EnsureVisibleLive(ctx, tx, "contact", id)
}
