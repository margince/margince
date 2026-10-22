// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"slices"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func previewFieldsVisible(ctx context.Context, object string, fields ...string) error {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	for _, field := range auth.MaskedFields(actor, object, true) {
		if slices.Contains(fields, field) {
			return apperrors.ErrPermissionDenied
		}
	}
	return nil
}
