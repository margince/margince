// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// PublicationAuthority fences authority and erasure before reading the accountable human.
func (s *Service) PublicationAuthority(ctx context.Context, tx pgx.Tx, user ids.UUID) (authz.RBAC, principal.SeatType, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return authz.RBAC{}, "", err
	}
	if err := storekit.FenceReportProjection(ctx, tx); err != nil {
		return authz.RBAC{}, "", err
	}
	var seat principal.SeatType
	err := tx.QueryRow(ctx, "SELECT seat_type FROM app_user WHERE id="+storekit.Placeholders([]ids.UUID{user})+" AND "+LiveMemberSQL(""), user).Scan(&seat)
	if errors.Is(err, pgx.ErrNoRows) {
		return authz.RBAC{}, "", apperrors.ErrNotFound
	}
	if err != nil {
		return authz.RBAC{}, "", err
	}
	_, teams, permissions, err := loadGrants(ctx, tx, ids.From[ids.UserKind](user))
	return authz.RBAC{Permissions: permissions, TeamIDs: rawTeamIDs(teams)}, seat, err
}
