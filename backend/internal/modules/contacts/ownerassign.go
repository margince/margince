// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ensureHandedOnOwnerAssignable refuses an edit that hands a contact or company
// to somebody the caller may not hand work to — the rule a deal's owner change
// already asks (auth.EnsureAssignee).
//
// Only a CHANGE of owner is an assignment. A form that sends the owner the
// record already has back unchanged hands nothing on, and refusing it would
// refuse every save of a record whose owner has since been suspended.
func ensureHandedOnOwnerAssignable(ctx context.Context, tx pgx.Tx, current *openapi_types.UUID, next *ids.UserID) error {
	if next == nil {
		return nil
	}
	if current != nil && ids.UUID(*current) == next.UUID {
		return nil
	}
	return auth.EnsureAssignee(ctx, tx, next.UUID)
}
