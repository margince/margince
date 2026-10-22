// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// The second boundary BYO-RES-2's warning covers. The row-scope arm is
// noteRowScope, beside the envelope it rides.

// noteCaptureWithheld raises BYO-RES-2's warning for the boundary row scope
// does not describe.
//
// It reuses the code and the words, because what a reader must not conclude is
// the same. An answer covers what their access allows and others may exist. A
// second code would ask a client to learn which boundary bounded it, which the
// disclosure rule withholds.
//
// Unlike the row-scope arm this cannot be answered before the read, so an
// answer nothing withheld stays silent and the warning keeps discriminating.
func noteCaptureWithheld(ctx context.Context, withheld *auth.WithheldReport) {
	if !withheld.Withheld() {
		return
	}
	noteWarning(ctx, warningRowScopeFiltered, rowScopeFilteredMessage)
}
