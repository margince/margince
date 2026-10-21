// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// ProbeCounterpartyMatch refuses an address that already belongs to a contact
// the caller may not file onto. Ensuring it on the caller's say-so files mail
// onto that contact and promotes it to the workspace, so a read-only share is
// not enough.
func (s *Store) ProbeCounterpartyMatch(ctx context.Context, email string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return s.ProbeCounterpartyMatchTx(ctx, tx, email)
	})
}

// ProbeCounterpartyMatchTx is ProbeCounterpartyMatch on the caller's
// transaction. It matches the address as ensureContact does, and holds each
// matched contact FOR SHARE until the ensure that follows has written.
func (s *Store) ProbeCounterpartyMatchTx(ctx context.Context, tx pgx.Tx, email string) error {
	matched, err := exactContactByEmail(ctx, tx, []string{email})
	if err != nil {
		return err
	}
	for _, id := range matched {
		if err := auth.EnsureAttachTarget(ctx, tx, entityContact, id.UUID); err != nil {
			return err
		}
	}
	return nil
}
