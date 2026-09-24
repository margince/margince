// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mailDraftRetentionActor is the principal the pass's audit rows carry.
const mailDraftRetentionActor = "system:mail_draft_retention"

// MailDraftRetentionArgs schedules one purge of drafts nobody has saved within
// activities.MailDraftRetention.
type MailDraftRetentionArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (MailDraftRetentionArgs) Kind() string { return "mail_draft_retention" }

// InsertOpts carries the attempt cap the declaration publishes, because the
// periodic insert supplies uniqueness and no attempt policy of its own.
func (MailDraftRetentionArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 3,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	}
}

type mailDraftRetentionWorker struct {
	drafts   *activities.Store
	identity *identity.Service
	log      *slog.Logger
}

func (w *mailDraftRetentionWorker) Work(ctx context.Context, _ *river.Job[MailDraftRetentionArgs]) error {
	wsCtx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	wsCtx = principal.SystemActing(wsCtx, mailDraftRetentionActor)
	purged, err := w.drafts.PurgeStaleMailDrafts(wsCtx)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if purged > 0 {
		w.log.InfoContext(wsCtx, "mail draft retention: untouched drafts purged", "drafts", purged)
	}
	return nil
}
