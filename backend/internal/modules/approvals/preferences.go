// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	preferenceMutedKey       = "muted"
	kindLifecycleChange      = "lifecycle_change"
	preferenceReaderKey      = "reader"
	kindCompanyNamePromotion = "company_name_promotion"
)

// New kinds need an explicit review before a personal preference may hide them.
func muteableKinds() []string {
	return []string{closeDateCorrectionKind, kindCompanyNamePromotion, kindLifecycleChange, kindDealFollowUp, kindCommitmentTask}
}

// ApprovalPreferences keeps personal choices separate from shared proposal status.
func (s *Service) ApprovalPreferences(ctx context.Context) ([]crmcontracts.ApprovalPreference, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, err
	}
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID.IsZero() {
		return nil, apperrors.ErrPermissionDenied
	}
	var out []crmcontracts.ApprovalPreference
	err := s.db.Tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = preferencesInTx(ctx, tx, p.UserID)
		return err
	})
	return out, err
}

func preferencesInTx(ctx context.Context, tx pgx.Tx, reader ids.UUID) ([]crmcontracts.ApprovalPreference, error) {
	rows, err := tx.Query(ctx, `SELECT kind, muted FROM approval_preference WHERE user_id = @reader`, pgx.NamedArgs{preferenceReaderKey: reader})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stored := make(map[string]bool)
	for rows.Next() {
		var kind string
		var muted bool
		if err := rows.Scan(&kind, &muted); err != nil {
			return nil, err
		}
		stored[kind] = muted
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]crmcontracts.ApprovalPreference, 0, len(muteableKinds()))
	for _, kind := range muteableKinds() {
		out = append(out, crmcontracts.ApprovalPreference{Kind: kind, Muted: stored[kind]})
	}
	return out, nil
}

// SetApprovalPreference makes either preference change return automatic writes to manual review.
func (s *Service) SetApprovalPreference(ctx context.Context, kind string, muted bool) ([]crmcontracts.ApprovalPreference, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, err
	}
	if !slices.Contains(muteableKinds(), kind) {
		return nil, apperrors.ErrNotFound
	}
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID.IsZero() {
		return nil, apperrors.ErrPermissionDenied
	}
	var out []crmcontracts.ApprovalPreference
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockPersonalPolicy(ctx, tx, p.UserID, kind); err != nil {
			return err
		}
		before, err := preferenceMuted(ctx, tx, p.UserID, kind)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO approval_preference (user_id, kind, muted)
   VALUES (@reader, @kind, @muted) ON CONFLICT (user_id, kind) DO UPDATE SET muted = EXCLUDED.muted`,
			pgx.NamedArgs{preferenceReaderKey: p.UserID, approvalKeyKind: kind, preferenceMutedKey: muted}); err != nil {
			return err
		}
		beforeAudit := map[string]any{"approval_kind": kind, preferenceMutedKey: before}
		afterAudit := map[string]any{"approval_kind": kind, preferenceMutedKey: muted}
		if before != muted {
			if err := resetPreferenceAutonomy(ctx, tx, p.UserID, kind, beforeAudit, afterAudit); err != nil {
				return err
			}
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "user", p.UserID,
			beforeAudit, afterAudit)
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, p.UserID, crmcontracts.PublicEventApprovalPreferenceChanged{Kind: kind}); err != nil {
			return err
		}
		out, err = preferencesInTx(ctx, tx, p.UserID)
		return err
	})
	return out, err
}

func preferenceMuted(ctx context.Context, tx pgx.Tx, reader ids.UUID, kind string) (bool, error) {
	var muted bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM approval_preference WHERE user_id = @reader AND kind = @kind AND muted)`, pgx.NamedArgs{preferenceReaderKey: reader, approvalKeyKind: kind}).Scan(&muted)
	return muted, err
}

// Mute and automatic application must not race into a hidden automatic-write policy.
func lockPersonalPolicy(ctx context.Context, tx pgx.Tx, reader ids.UUID, kind string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(@policy, 0))`, pgx.NamedArgs{"policy": fmt.Sprintf("approval-policy:%s:%s", reader, kind)})
	return err
}

func writeAutonomyMode(ctx context.Context, tx pgx.Tx, reader ids.UUID, kind string, mode AutonomyMode) error {
	_, err := tx.Exec(ctx, `INSERT INTO approval_autonomy_policy (user_id, kind, mode)
 VALUES (@reader, @kind, @mode) ON CONFLICT (user_id, kind) DO UPDATE SET mode = EXCLUDED.mode, veto_window = NULL`,
		pgx.NamedArgs{preferenceReaderKey: reader, approvalKeyKind: kind, "mode": string(mode)})
	return err
}

func resetPreferenceAutonomy(ctx context.Context, tx pgx.Tx, reader ids.UUID, kind string, before, after map[string]any) error {
	if !AutoApplyKinds[kind] {
		return nil
	}
	mode := unsetAutonomy(kind)
	err := tx.QueryRow(ctx, `SELECT mode FROM approval_autonomy_policy WHERE user_id = @reader AND kind = @kind`, pgx.NamedArgs{preferenceReaderKey: reader, approvalKeyKind: kind}).Scan(&mode)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	before["mode"] = mode
	after["mode"] = ModeManual
	return writeAutonomyMode(ctx, tx, reader, kind, ModeManual)
}
