// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SourceAuthority rebuilds the assignee's authority before source evidence is used.
type SourceAuthority func(context.Context, pgx.Tx, ids.UUID) (principal.Principal, bool, error)

// WithSourceAuthority shares current assignee authority with source-evidence checks.
func (s *Store) WithSourceAuthority(read SourceAuthority) *Store { s.sourceAuthority = read; return s }

// WithSourceAuthority shares current assignee authority with source-evidence checks.
func (h Handlers) WithSourceAuthority(read SourceAuthority) Handlers {
	h.store.WithSourceAuthority(read)
	return h
}

// WithSourceAuthority shares current assignee authority with source-evidence checks.
func (p *Provider) WithSourceAuthority(read SourceAuthority) *Provider {
	p.store.WithSourceAuthority(read)
	return p
}

func sourceHolder(ctx context.Context, tx pgx.Tx, holder ids.UUID, read SourceAuthority) (principal.Principal, error) {
	if read == nil {
		return auth.SeatPrincipal(ctx, tx, holder)
	}
	member, live, err := read(ctx, tx, holder)
	if err != nil {
		return principal.Principal{}, err
	}
	if !live {
		return principal.Principal{}, apperrors.ErrNotFound
	}
	return member, nil
}
