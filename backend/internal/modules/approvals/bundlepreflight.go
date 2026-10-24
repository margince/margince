// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Service) preflightBundle(ctx context.Context, p principal.Principal, bundleID ids.UUID, approve bool) (map[ids.ApprovalID]bool, error) {
	var mine []row
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, oversized, err := bundleMembers(ctx, tx, bundleID, false)
		if err != nil {
			return err
		}
		mine, err = s.decidableMembers(ctx, tx, p, rows, approve)
		if err != nil {
			return err
		}
		if len(mine) == 0 {
			return apperrors.ErrNotFound
		}
		if oversized {
			return &BundleTooLargeError{Cap: bundleDecisionCap}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	ready := make(map[ids.ApprovalID]bool, len(mine))
	if !approve {
		return ready, nil
	}
	for _, a := range mine {
		if a.effectiveStatus(s.now()) != statusPending {
			continue
		}
		if err := s.precheckMember(ctx, a, nil); err != nil {
			if decisionContextRefusal(err) {
				continue
			}
			return nil, err
		}
		ready[a.ID] = true
	}
	return ready, nil
}

// Prechecks may open their own transactions, so they precede bundle row locks.
// Membership and authority are checked again under lock; new members stay pending.
func (s *Service) precheckMember(ctx context.Context, a row, edited json.RawMessage) error {
	if err := s.checkPreview(ctx, a, edited); err != nil {
		return err
	}
	// Before the kind's own check, which reads the edit as if it were allowed.
	if edited != nil {
		if err := assertEditableMembers(a.Kind, a.ProposedChange, edited); err != nil {
			return err
		}
	}
	check, ok := s.prechecks[a.Kind]
	if !ok || !serverProposed(a) {
		return nil
	}
	return check(ctx, a.ProposedChange, edited)
}

func decisionContextRefusal(err error) bool {
	var field apperrors.FieldFault
	var fields apperrors.FieldFaults
	var message apperrors.MessageFault
	return errors.As(err, &field) || errors.As(err, &fields) || errors.As(err, &message) ||
		errors.Is(err, apperrors.ErrVersionSkew) || errors.Is(err, apperrors.ErrNotFound) ||
		errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrInvalidArgument) || errors.Is(err, apperrors.ErrConflict)
}
