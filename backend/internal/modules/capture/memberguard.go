// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MemberGuard re-reads, inside the capture transaction, what let a member's
// connection file. A caller that resolved consent and authority earlier passes
// it. A credential or member removed in between then refuses the capture, and
// nothing is stored.
type MemberGuard func(ctx context.Context, tx pgx.Tx, member ids.UUID) error

type memberGuardKey struct{}

// WithMemberGuard marks a capture as needing the guard to pass first.
func WithMemberGuard(ctx context.Context, guard MemberGuard) context.Context {
	return context.WithValue(ctx, memberGuardKey{}, guard)
}

// memberGuardTx runs the guard a caller attached, or passes when none is.
func memberGuardTx(ctx context.Context, tx pgx.Tx, member ids.UUID) error {
	if guard, ok := ctx.Value(memberGuardKey{}).(MemberGuard); ok {
		return guard(ctx, tx, member)
	}
	return nil
}
