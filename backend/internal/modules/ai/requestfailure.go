// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The rail's answer for work that failed before the router was entered.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// AnnounceRequestFailure traces a request that failed BEFORE this router was
// ever entered — a caller-side preparation step (assembling company context)
// that died on work a user asked for. Without it the failure is invisible on
// the AI-activity rail exactly like the pre-trace returns inside serveAttempt
// used to be: the user saw an error and the rail said nothing ran. It mints
// its own logical call and flushes the one failed trace through the same
// detached path every served call uses, so the settle announce and the
// projection see an ordinary failed occurrence.
func (r *Router) AnnounceRequestFailure(ctx context.Context, task Task, cause error) {
	lc := newLogicalCall()
	trace := r.newAttemptTrace(ctx, task, "", "", model.Request{})
	trace.ErrorSentinel = classifyError(cause)
	lc.append(trace)
	r.flushDetached(ctx, r.binding(), lc)
}
