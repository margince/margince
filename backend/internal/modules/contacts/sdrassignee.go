// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Submission proposes a receiver; acceptance still requires the actual prospect write grant.
// Reassignment also checks that the successor can act on the prospect.
func requireHandoffAssignee(ctx context.Context, tx pgx.Tx, receiver *ids.UUID) error {
	if receiver == nil {
		return nil
	}
	err := auth.EnsureHandOverRecipient(ctx, tx, *receiver)
	if errors.As(err, new(*auth.AssigneeNotAllowedError)) {
		return &values.ParseError{Field: fieldHandoffAssignee, Code: "not_assignable", Message: "choose an active colleague who can take this handoff"}
	}
	return err
}
