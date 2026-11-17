// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
)

// WithInstallationLock refuses every tool call while the installation is
// locked, reads included: the agent stops working with the installation. lock
// answers apperrors.ErrTrialEnded or apperrors.ErrLicenseLapsed when locked.
func WithInstallationLock(lock func(context.Context) error) RegistryOption {
	return func(r *Registry) { r.installationLock = lock }
}

func (r *Registry) refuseWhenLocked(ctx context.Context) error {
	if r.installationLock == nil {
		return nil
	}
	return r.installationLock(ctx)
}
