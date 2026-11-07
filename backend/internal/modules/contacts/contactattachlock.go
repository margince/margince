// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Attaching an edge holds its parent live without granting authority to change that parent.
func lockContactForAttach(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) error {
	return auth.LockSubjectLive(ctx, tx, contactEntity, contactID.UUID)
}
