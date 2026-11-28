// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PurposeInUseError keeps the refusal readable without exposing its references.
type PurposeInUseError struct{}

func (*PurposeInUseError) Error() string {
	return "consent is already recorded for this purpose; create a new purpose instead"
}
func (*PurposeInUseError) Unwrap() error { return apperrors.ErrConflict }
func (e *PurposeInUseError) MessageFault() (string, string) {
	return "consent_purpose_in_use", e.Error()
}

func (s *Store) RenamePurpose(ctx context.Context, id ids.PurposeID, label string) (Purpose, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return Purpose{}, &ValidationError{Field: fieldLabel, Reason: "a label is required"}
	}
	return s.changeUnusedPurpose(ctx, id, &label)
}

func (s *Store) ArchivePurpose(ctx context.Context, id ids.PurposeID) error {
	_, err := s.changeUnusedPurpose(ctx, id, nil)
	return err
}

func (s *Store) changeUnusedPurpose(ctx context.Context, id ids.PurposeID, label *string) (Purpose, error) {
	if err := auth.Require(ctx, "consent_config", principal.ActionUpdate); err != nil {
		return Purpose{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return Purpose{}, apperrors.ErrPermissionDenied
	}
	var out Purpose
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id,key,label,requires_double_opt_in,class,frequency,created_at
    FROM consent_purpose WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, id).
			Scan(&out.ID, &out.Key, &out.Label, &out.RequiresDoubleOptIn, &out.Class, &out.Frequency, &out.CreatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		before := map[string]any{fieldLabel: out.Label, "archived_at": nil}
		action := "renamed"
		var archived *time.Time
		if label == nil {
			action = "archived"
			now := s.now().UTC()
			archived = &now
		} else if *label == out.Label {
			return nil
		} else if err := lockPurposeLabels(ctx, tx, *label, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE consent_purpose SET label=coalesce($2,label),archived_at=$3 WHERE id=$1`, id, label, archived); err != nil {
			return err
		}
		if label != nil {
			out.Label = *label
		}
		return auditPurposeChange(ctx, tx, id, action, before, map[string]any{fieldLabel: out.Label, "archived_at": archived})
	})
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.ConstraintName == "consent_purpose_in_use" {
		return Purpose{}, &PurposeInUseError{}
	}
	return out, err
}

func auditPurposeChange(ctx context.Context, tx pgx.Tx, id ids.PurposeID, action string, before, after map[string]any) error {
	auditAction := "update"
	if action == "created" {
		auditAction = "create"
	}
	audit, err := storekit.Audit(ctx, tx, auditAction, "consent_purpose", id.UUID, before, after)
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, audit, id.UUID, crmcontracts.InternalEventConsentPurposeChanged{Action: crmcontracts.InternalEventConsentPurposeChangedAction(action)})
}
