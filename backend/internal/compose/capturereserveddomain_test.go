// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// A booted role whose file says nothing skips reserved-domain proposals. The
// zero CaptureConfig does not, which is what hand-built test configs rely on.
func TestABootedRoleSkipsReservedDomainProposalsByDefault(t *testing.T) {
	cfg := CaptureConfigFromDeploy(deployconfig.Capture{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if !cfg.SkipReservedDomainProposals {
		t.Error("CaptureConfigFromDeploy on a silent capture block left SkipReservedDomainProposals off, want on")
	}
	if (CaptureConfig{}).SkipReservedDomainProposals {
		t.Error("the zero CaptureConfig skips reserved-domain proposals, want off")
	}
}
