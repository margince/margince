// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unlessDenied answers nil for a permission refusal, which only means this
// caller cannot promote, and the error itself for anything else.
func unlessDenied(err error) error {
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	return err
}

// mayPromoteHeldLeads answers whether the caller holds both grants a lead
// promotion needs, so a refusal can say whether converting is on offer.
func mayPromoteHeldLeads(ctx context.Context) (bool, error) {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return false, unlessDenied(err)
	}
	if err := auth.Require(ctx, "contact", principal.ActionCreate); err != nil {
		return false, unlessDenied(err)
	}
	return true, nil
}

// MayPromoteHeldLeads answers whether a human may authorize cleanup of an
// existing lead and contact pair by a system-run write.
func MayPromoteHeldLeads(ctx context.Context) (bool, error) {
	return mayPromoteHeldLeads(ctx)
}
