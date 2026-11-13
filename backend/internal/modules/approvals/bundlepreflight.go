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
		var visible int
		mine, visible, err = s.decidableMembers(ctx, tx, p, rows, approve)
		if err != nil {
			return err
		}
		if len(mine) == 0 {
			return hiddenBundle(ctx, len(rows), visible)
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
			if obsolete, ok := errors.AsType[*ObsoleteError](err); ok {
				// Closed here as it is when decided alone, not left pending.
				if err := s.withdrawObsolete(ctx, a.ID, obsolete); !errors.Is(err, obsolete) {
					return nil, err
				}
				continue
			}
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
		if obsolete := s.obsoleteByKind(ctx, a, err); obsolete != nil {
			return obsolete
		}
		return err
	}
	// Before the kind's own check, which reads the edit as if it were allowed.
	if edited != nil {
		if err := assertEditableMembers(a.Kind, a.ProposedChange, edited); err != nil {
			return err
		}
	}
	if agentCallNeedsRecheck(a) {
		if edited != nil {
			a.ProposedChange = edited
		}
		return s.precheckAgentCall(ctx, a)
	}
	check, ok := s.prechecks[a.Kind]
	if !ok || !serverProposed(a) {
		return nil
	}
	return check(ctx, a.ProposedChange, edited)
}

// obsoleteByKind is the kind's own reason for a card its preview found stale,
// when that reason is that the work is no longer needed. It is the more
// useful answer: the card closes with it instead of waiting with "version
// skew". It reads the staged payload only, before any edit is admitted.
func (s *Service) obsoleteByKind(ctx context.Context, a row, stale error) *ObsoleteError {
	check, ok := s.prechecks[a.Kind]
	if !ok || !serverProposed(a) || !errors.Is(stale, apperrors.ErrVersionSkew) {
		return nil
	}
	obsolete, _ := errors.AsType[*ObsoleteError](check(ctx, a.ProposedChange, nil))
	return obsolete
}

func decisionContextRefusal(err error) bool {
	var field apperrors.FieldFault
	var fields apperrors.FieldFaults
	var message apperrors.MessageFault
	return errors.As(err, &field) || errors.As(err, &fields) || errors.As(err, &message) ||
		errors.Is(err, apperrors.ErrVersionSkew) || errors.Is(err, apperrors.ErrNotFound) ||
		errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrConsentNotGranted) || errors.Is(err, apperrors.ErrSeatTierInsufficient) || errors.Is(err, apperrors.ErrInvalidArgument) || errors.Is(err, apperrors.ErrConflict)
}
