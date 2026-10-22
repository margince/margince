// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MailSeatIdentitiesSQL binds private identities for matching readable mail only.
func (s *OwnDomainStore) MailSeatIdentitiesSQL(ctx context.Context, tx pgx.Tx, bind func(any) int) (string, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return "", err
	}
	return storekit.MailSeatIdentitiesSQL(ctx, tx, bind)
}

func mailSeatIdentities(ctx context.Context, tx pgx.Tx) (map[string][]string, error) {
	return storekit.MailSeatIdentities(ctx, tx)
}
