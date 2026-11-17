// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// InstallationLock answers why the installation is locked: ErrTrialEnded,
// ErrLicenseLapsed, nil when it is not, or another error for a failed read. The
// composition root answers it from the license posture and the workspace's
// creation time, neither of which this module owns.
type InstallationLock func(context.Context) error

// WithInstallationLock installs the check every admission door runs before a
// mutation, and that /me reports so every seat sees the lock, not only admins.
func (h Handlers) WithInstallationLock(lock InstallationLock) Handlers {
	h.installationLock = lock
	return h
}

// refuseWhenLocked answers the lock's error for a mutation it stops, and nil for
// everything else. Reads stay open, so the data stays reachable.
func (h Handlers) refuseWhenLocked(ctx context.Context, r *http.Request) error {
	if h.installationLock == nil || !isMutating(r.Method) || mayMutateWhenLocked(r) {
		return nil
	}
	return h.installationLock(ctx)
}

// lockForMe answers /me's two flags. A failed read reports no lock: /me is how
// a seat signs in, and the mutation gate still refuses.
func (h Handlers) lockForMe(ctx context.Context) (trialEnded, licenseLapsed *bool) {
	if h.installationLock == nil {
		return nil, nil
	}
	locked := true
	switch err := h.installationLock(ctx); {
	case errors.Is(err, apperrors.ErrTrialEnded):
		return &locked, nil
	case errors.Is(err, apperrors.ErrLicenseLapsed):
		return nil, &locked
	}
	return nil, nil
}

// The mutations a locked installation still admits. A seat's own credentials (the
// read-seat list) and the license that ends the lock keep a way out. A data
// export and data-subject requests are owed whether or not it pays.
var (
	licensePathPrefix            = httpserver.BaseURL + "/installation/license"
	exportPath                   = httpserver.BaseURL + "/exports"
	dataSubjectRequestPathPrefix = httpserver.BaseURL + "/data-subject-requests"
)

func mayMutateWhenLocked(r *http.Request) bool {
	path := r.URL.Path
	return readSeatMayMutate(r) ||
		strings.HasPrefix(path, licensePathPrefix) ||
		(r.Method == http.MethodPost && path == exportPath) ||
		strings.HasPrefix(path, dataSubjectRequestPathPrefix)
}
