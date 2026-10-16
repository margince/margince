// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The retain-only posture's own read/write surface (GCS-WIRE-5, GCS-PARAM-6),
// shaped like capture's settings store. The module owns the sparse-patch
// semantics and the RBAC gate on the write. The configuration owns the boot
// lock, and platform/settings the row, the validation and the audit entry.
//
// The evaluator and the policy store do not come through here. They read the
// posture with retainOnlyTx inside a transaction they already hold. A pass that
// read it in a second transaction could act on some records under one posture
// and the rest under another.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PostureStore reads and writes the installation's retain-only posture.
type PostureStore struct {
	settings *config.Config
}

// NewPostureStore wires the posture over the configuration, which reads and
// saves it under the boot locks it holds.
func NewPostureStore(cfg *config.Config) *PostureStore {
	return &PostureStore{settings: cfg}
}

// Posture reads whether the installation destroys nothing.
func (s *PostureStore) Posture(ctx context.Context) (bool, error) {
	retainOnly, err := config.StoredVal(ctx, s.settings, RetainOnly)
	if err != nil {
		return false, fmt.Errorf("privacy: reading the retention posture: %w", err)
	}
	return retainOnly, nil
}

// SetPosture applies a sparse posture patch. A nil value is a patch that names
// no field: it changes nothing and answers with the current posture, so an
// idempotent retry is not an error. Returns the posture after the write.
func (s *PostureStore) SetPosture(ctx context.Context, retainOnly *bool) (bool, error) {
	// Gated here as well as inside config.Save. The nil branch below never
	// reaches that write, so without this an update-shaped request would pass
	// on the read grant alone.
	if err := auth.Require(ctx, retentionPolicyObject, principal.ActionUpdate); err != nil {
		return false, err
	}
	if retainOnly == nil {
		return s.Posture(ctx)
	}
	if err := config.Save(ctx, s.settings, RetainOnly, *retainOnly); err != nil {
		return false, err
	}
	return *retainOnly, nil
}
