// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Whether this installation has described its own company, asked before any
// seat is added. The anchor company is contacts' row, which identity never
// imports, so the composition root injects the answer.

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// InstallationDescribed answers whether this installation has saved its own
// company — the anchor the onboarding company form writes.
type InstallationDescribed func(context.Context) (bool, error)

// WithInstallationDescribed binds the answer on the SERVICE, as WithSeatCeiling
// does, so every surface reaching the seat writers is held to it.
func (h Handlers) WithInstallationDescribed(described InstallationDescribed) Handlers {
	if h.svc == nil {
		return h
	}
	h.svc.installationDescribed = described
	return h
}

var (
	errCompanyNotDescribed = fmt.Errorf("%w: this installation has not described its own company", apperrors.ErrConflict)
	// Unwired is a composition fault, not the refusal above: telling an admin to
	// describe a company they already saved would send them to the wrong fix.
	errInstallationDescribedUnwired = errors.New("identity: nothing answers whether the installation has described its company")
)

// refuseUntilDescribed: an installation adds nobody until it has described itself,
// because the client treats every non-admin seat as proof that it has. Asked outside
// the seat's transaction, since an anchor is never archived or merged.
func (s *Service) refuseUntilDescribed(ctx context.Context) error {
	if s.installationDescribed == nil {
		return errInstallationDescribedUnwired
	}
	described, err := s.installationDescribed(ctx)
	if err != nil {
		return err
	}
	if !described {
		return errCompanyNotDescribed
	}
	return nil
}
