// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MemberGrantWithdrawal ends provider grants in their owner's module before departure commits.
type MemberGrantWithdrawal func(context.Context, pgx.Tx, ids.UserID) error

// WithMemberGrantWithdrawal revokes personal messaging grants with member withdrawal.
func (s *Service) WithMemberGrantWithdrawal(withdraw MemberGrantWithdrawal) *Service {
	s.memberGrantWithdrawal = withdraw
	return s
}

// WithMemberGrantWithdrawal revokes personal messaging grants with member withdrawal.
func (h Handlers) WithMemberGrantWithdrawal(withdraw MemberGrantWithdrawal) Handlers {
	if h.svc != nil {
		h.svc.WithMemberGrantWithdrawal(withdraw)
	}
	return h
}
