// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func consentEntityName(ctx context.Context, tx pgx.Tx, id ids.UUID) (string, error) {
	entity, err := contacts.ResolveEntityIdentity(ctx, tx, ids.From[ids.LegalEntityKind](id))
	if err != nil {
		return "", err
	}
	return entity.DisplayName, nil
}
