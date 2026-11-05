// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
)

// gatherInstallation reads the installation settings a card is written from.
// The language is the one the card is worded in. The zone is the one every day
// count on the card is reckoned in, the zone the reader sees the dates in.
func (s *Service) gatherInstallation(ctx context.Context, f *facts) error {
	f.lang = identity.BaseLanguageForPrompt(ctx, s.pool)
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		loc, err := identity.ZoneAppliedTx(ctx, tx)
		f.loc = loc
		return err
	})
	if err != nil {
		return fmt.Errorf("deal status: reading the installation's timezone: %w", err)
	}
	return nil
}
