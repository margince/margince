// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// readMemberLanguages is the UI languages active members chose, each once and
// with no member named: what an admin-named label needs a name in, without
// telling anyone which colleague reads which language.
func (s *InstallationSettingsStore) readMemberLanguages(ctx context.Context, out *InstallationSettings) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT locale FROM app_user
			 WHERE `+LiveMemberSQL("")+` AND NOT is_agent AND locale IS NOT NULL
			 ORDER BY locale`)
		if err != nil {
			return fmt.Errorf("reading the members' languages: %w", err)
		}
		languages, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("reading the members' languages: %w", err)
		}
		out.MemberLanguages = languages
		return nil
	})
}
