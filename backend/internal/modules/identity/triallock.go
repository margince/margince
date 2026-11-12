// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// TrialEnded reports whether the installation's trial is over. The composition
// root answers it from the license posture and the workspace's creation time,
// neither of which this module owns.
type TrialEnded func(context.Context) (bool, error)

// WithTrialLock installs the check every admission door runs before a
// mutation, and that /me reports so every seat sees the lock, not only admins.
func (h Handlers) WithTrialLock(ended TrialEnded) Handlers {
	h.trialEnded = ended
	return h
}

// refuseAfterTrial answers ErrTrialEnded for a mutation the ended trial stops,
// and nil for everything else. Reads stay open, so the data stays reachable.
func (h Handlers) refuseAfterTrial(ctx context.Context, r *http.Request) error {
	if h.trialEnded == nil || !isMutating(r.Method) || mayMutateAfterTrial(r) {
		return nil
	}
	ended, err := h.trialEnded(ctx)
	if err != nil {
		return err
	}
	if ended {
		return apperrors.ErrTrialEnded
	}
	return nil
}

// trialEndedForMe answers /me's flag. A failed read reports the trial as not
// ended: /me is how a seat signs in, and the mutation gate still refuses.
func (h Handlers) trialEndedForMe(ctx context.Context) *bool {
	if h.trialEnded == nil {
		return nil
	}
	ended, err := h.trialEnded(ctx)
	if err != nil || !ended {
		return nil
	}
	return &ended
}

// The mutations an ended trial still admits. A seat's own credentials (the
// read-seat list) and the license that ends the lock keep a way out. A data
// export and data-subject requests are owed whether or not it pays.
var (
	licensePathPrefix            = httpserver.BaseURL + "/installation/license"
	exportPath                   = httpserver.BaseURL + "/exports"
	dataSubjectRequestPathPrefix = httpserver.BaseURL + "/data-subject-requests"
)

func mayMutateAfterTrial(r *http.Request) bool {
	path := r.URL.Path
	return readSeatMayMutate(r) ||
		strings.HasPrefix(path, licensePathPrefix) ||
		(r.Method == http.MethodPost && path == exportPath) ||
		strings.HasPrefix(path, dataSubjectRequestPathPrefix)
}
