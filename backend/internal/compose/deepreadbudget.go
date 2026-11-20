// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/providerwait"
)

func (w *siteDeepReadWorker) Work(ctx context.Context, job *river.Job[SiteDeepReadArgs]) (workErr error) {
	// River recovers a worker panic, but it cannot close this module's
	// claimed dossier. Recover at the ownership boundary so an unexpected
	// provider/parser panic becomes a terminal failed read instead of a row
	// that tells the browser "reading" forever.
	if _, err := workspaceJobCtx(ctx, job.Args); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	workCtx := deepReadWorkerCtx(ctx, job.Args)
	defer func() {
		if recovered := recover(); recovered != nil {
			cause := fmt.Errorf("site deep read panic: %v", recovered)
			if w.log != nil {
				w.log.ErrorContext(workCtx, "site deep read panic recovered",
					"read", job.Args.SiteReadID.String(), "panic", recovered, "stack", string(debug.Stack()))
			}
			workErr = jobs.FaultContext(ctx, w.fail(workCtx, job.Args.SiteReadID, cause))
		}
	}()
	err := w.run(workCtx, job.Args)
	until, deferred := ai.DeferredUntil(err)
	if !deferred {
		return jobs.FaultContext(ctx, err)
	}
	now := time.Now()
	if w.now != nil {
		now = w.now()
	}
	delay := max(until.Sub(now), 0)
	return river.JobSnooze(delay)
}

func (w *siteDeepReadWorker) deferForBudget(ctx context.Context, readID ids.UUID, cause error) (bool, error) {
	until, deferred := ai.DeferredUntil(cause)
	if !deferred {
		return false, nil
	}
	tctx, cancel := terminalCtx(ctx)
	defer cancel()
	detail := contacts.SiteReadBudgetDetail
	if errors.Is(cause, ai.ErrProviderDown) {
		detail = providerwait.Detail
	}
	if err := w.contacts.DeferSiteRead(tctx, readID, until, detail); err != nil {
		return true, errors.Join(cause, fmt.Errorf("recording the deferral on the dossier: %w", err))
	}
	return true, cause
}
