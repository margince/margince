// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// refuseRenameTx admits only the name the installation already holds. The
// anchor company's display name is that name, and a rename here would drift
// from it. An older client resending the name it read still saves.
func refuseRenameTx(ctx context.Context, tx pgx.Tx, c *config.Config, next *string) error {
	if next == nil {
		return nil
	}
	if err := settings.LockForWrite(ctx, tx, Name.Key()); err != nil {
		return err
	}
	current, err := config.StoredValTx(ctx, tx, c, Name)
	if err != nil {
		return err
	}
	if *next == current {
		return nil
	}
	return c.LockFirst(Name.Key(), settings.InvalidValue{
		Setting: Name.Key(), Code: "set_on_company_profile",
		Reason: "the installation's name is the company's name; change it on the company profile",
	})
}
