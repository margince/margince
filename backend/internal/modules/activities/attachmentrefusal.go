// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// requireParentOrHide checks the parent object grant after the attachment row
// was found, so it exists in this workspace. A caller lacking the grant must
// not learn the attachment exists, so object denial reads as not-found: the
// same 404 a row-scope miss returns. Upload checks the grant before any
// lookup, so it keeps its plain 403.
func requireParentOrHide(ctx context.Context, entityType string, action principal.Action) error {
	if err := auth.Require(ctx, entityType, action); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return apperrors.ErrNotFound
		}
		return err
	}
	return nil
}

// deniedChange answers a caller refused a change to a file. One who may read
// its record is told the change is not theirs, which they could see anyway.
// One who may not learns nothing.
func deniedChange(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID) error {
	if err := requireParentOrHide(ctx, entityType, principal.ActionRead); err != nil {
		return err
	}
	if err := ensureAttachmentParentVisible(ctx, tx, entityType, entityID); err != nil {
		return err
	}
	return apperrors.ErrPermissionDenied
}
