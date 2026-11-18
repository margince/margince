// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// inlineSendReview reads the composer's pilot switch for /me, through the
// setting's own read gate. A seat that may not read installation settings is
// answered off, so /me still answers it.
func inlineSendReview(pool *pgxpool.Pool) identity.InlineSendReview {
	return func(ctx context.Context) (bool, error) {
		on, err := config.StoredVal(ctx, StoredConfig(pool), activities.InlineSendReview)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		return on, err
	}
}
