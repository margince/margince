// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The rail's answer for work that failed before the router was entered.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// AnnounceRequestFailure traces a request that failed BEFORE this router was
// ever entered — a caller-side preparation step (assembling company context)
// that died on work a user asked for. A failure of asked-for work reaches
// the rail wherever it happens; this is the entry point for the failures
// the router itself never sees. It mints its own logical call and flushes
// the one failed trace through the same detached path every served call
// uses, so the settle announce and the projection see an ordinary failed
// occurrence — labelled request_failed, because no provider was ever tried.
func (r *Router) AnnounceRequestFailure(ctx context.Context, task Task, cause error) {
	// The request's own correlation stays on the trace and the envelope:
	// one originating request is one correlation, and a synthetic scope
	// would sever the ai_call row and the occurrence from the story they
	// belong to. The cost is bounded churn — a sibling call of the same
	// task still running under this correlation can read failed until its
	// own settle repairs the occurrence — and the projection's attempt
	// guard makes that repair certain.
	lc := newLogicalCall()
	trace := r.newAttemptTrace(ctx, task, "", "", model.Request{})
	trace.ErrorSentinel = classifyError(fmt.Errorf("%w: %w", errRequestFailed, cause))
	lc.append(trace)
	r.flushDetached(ctx, r.binding(), lc)
}
