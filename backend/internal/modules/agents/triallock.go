// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// WithTrialLock refuses every tool call once the installation's trial has
// ended, reads included: the agent stops working with the installation.
func WithTrialLock(ended func(context.Context) (bool, error)) RegistryOption {
	return func(r *Registry) { r.trialEnded = ended }
}

func (r *Registry) refuseAfterTrial(ctx context.Context) error {
	if r.trialEnded == nil {
		return nil
	}
	ended, err := r.trialEnded(ctx)
	if err != nil {
		return err
	}
	if ended {
		return apperrors.ErrTrialEnded
	}
	return nil
}
