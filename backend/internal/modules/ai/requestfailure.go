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
	lc := newLogicalCall()
	trace := r.newAttemptTrace(ctx, task, "", "", model.Request{})
	trace.ErrorSentinel = classifyError(fmt.Errorf("%w: %w", errRequestFailed, cause))
	lc.append(trace)
	r.flushDetached(ctx, r.binding(), lc)
}
