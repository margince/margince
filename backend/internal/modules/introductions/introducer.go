// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func requireIntroducer(ctx context.Context, tx pgx.Tx, member ids.UUID) error {
	err := auth.EnsureHandOverRecipient(ctx, tx, member)
	if errors.As(err, new(*auth.AssigneeNotAllowedError)) {
		return &values.ParseError{Field: fieldIntroducerUserID, Code: "not_assignable", Message: "choose an active colleague who can answer this introduction request"}
	}
	return err
}
