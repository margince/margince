// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Lock the receiver before the case row, as member handover does.
func requireCaseAssignee(ctx context.Context, tx pgx.Tx, member ids.UUID, field string) error {
	err := auth.EnsureHandOverRecipient(ctx, tx, member)
	if errors.As(err, new(*auth.AssigneeNotAllowedError)) {
		return &ValidationError{Field: field, Reason: "choose an active colleague who can take this work"}
	}
	return err
}

func requireDSRAssignee(ctx context.Context, tx pgx.Tx, in UpdateDSRInput) error {
	if in.AssigneeID != nil && !in.ClearAssignee {
		return requireCaseAssignee(ctx, tx, in.AssigneeID.UUID, fieldAssigneeID)
	}
	return nil
}
