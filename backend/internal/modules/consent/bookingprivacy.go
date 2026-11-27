// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
)

// BookingPrivacyInformation publishes only the address the installation tells
// guests to use for privacy information. Other controller settings stay private.
func (s *Store) BookingPrivacyInformation(ctx context.Context) (string, error) {
	var address string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		particulars, err := config.StoredApplyTx(ctx, tx, config.Bound(), ControllerIdentity)
		if err != nil {
			return err
		}
		address = privacyPageOf(particulars)
		return nil
	})
	return address, err
}
