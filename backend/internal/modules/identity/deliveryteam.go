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
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	deliveryTeamRequired = requiredFieldCode
	deliveryTeamGone     = "delivery_team_gone"
	fieldRoutineMailTeam = "routine_mail_team_id"
)

// checkChosenTeam is validateDeliveryTeam for a save, where a team the caller
// can no longer review is a choice to correct on the field.
func checkChosenTeam(ctx context.Context, tx pgx.Tx, d Delivery) error {
	err := validateDeliveryTeam(ctx, tx, d)
	if errors.Is(err, apperrors.ErrNotFound) {
		return &values.ParseError{Field: fieldRoutineMailTeam, Code: deliveryTeamGone, Message: "this team no longer exists; choose another"}
	}
	return err
}

func validateDeliveryTeam(ctx context.Context, tx pgx.Tx, d Delivery) error {
	if d.TeamID == nil && (d.Scope == nil || *d.Scope == RoutinePersonal) {
		return nil
	}
	if d.TeamID == nil || d.TeamID.IsZero() {
		return &values.ParseError{Field: fieldRoutineMailTeam, Code: deliveryTeamRequired, Message: "choose a team for this view"}
	}
	switch auth.TeamWeekReachOf(ctx) {
	case auth.ReachesEveryTeam:
		var exists bool
		args := []any{*d.TeamID}
		if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM team WHERE id = "+storekit.Placeholders(args)+" AND archived_at IS NULL)", args...).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperrors.ErrNotFound
		}
		return nil
	case auth.ReachesTeamsLed:
		allowed, err := CallerLeadsLiveTeamTx(ctx, tx, *d.TeamID)
		if err != nil {
			return err
		}
		if allowed {
			return nil
		}
		return apperrors.ErrNotFound
	default:
		return apperrors.ErrPermissionDenied
	}
}

// DeliveryTeam resolves only a team the caller may currently review.
func (s *Service) DeliveryTeam(ctx context.Context, d Delivery) ([]TeamMember, bool, error) {
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error { return validateDeliveryTeam(ctx, tx, d) }); err != nil {
		return nil, false, err
	}
	if d.TeamID == nil {
		return nil, false, apperrors.ErrNotFound
	}
	return s.MembersOfTeam(ctx, *d.TeamID)
}

// DeliveryTeamName names only a team the caller may currently review.
func (s *Service) DeliveryTeamName(ctx context.Context, d Delivery) (string, error) {
	var name string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := validateDeliveryTeam(ctx, tx, d); err != nil {
			return err
		}
		if d.TeamID == nil {
			return apperrors.ErrNotFound
		}
		args := []any{*d.TeamID}
		return tx.QueryRow(ctx, "SELECT name FROM team WHERE id = "+storekit.Placeholders(args)+" AND archived_at IS NULL", args...).Scan(&name)
	})
	return name, err
}
