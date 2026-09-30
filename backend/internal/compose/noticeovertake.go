// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The pass that takes back the lines no event will ever take back.
//
// A staged approval is announced once per seat that could decide it, and five
// routes reach a terminal approval. Only a human decision and the expiry sweep
// emit anything; supersession, withdrawal and privacy erasure write the row and
// announce nothing, by explicit design against a closed event catalog. So a
// consumer covers two routes of five, and the other three leave every colleague
// who did not decide holding an unread line about a card their own inbox
// already hides.
//
// It lives here because the question crosses two modules — notices owns the
// line, approvals owns whether the card is still decidable — and no module
// imports a sibling.
//
// One job, not a dispatcher and a child (ADR-0103 §1): one scan over an
// installation-wide table, so there is nothing to fan out over.

import (
	"context"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// noticeOvertakeChunk bounds the WIDTH of one question to approvals, and
// nothing else. Every standing reference is asked about on every pass: a
// bounded READ would fill with the kinds expiry excludes — they never age out,
// so they stand for ever — and never reach what sorts behind them, reporting
// success with nothing failing.
const noticeOvertakeChunk = 1000

// NoticeOvertakeArgs takes back the lines about approvals nobody can decide.
type NoticeOvertakeArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (NoticeOvertakeArgs) Kind() string { return "notice_overtaken_sweep" }

// InsertOpts carries the attempt cap the declaration publishes, because the
// periodic insert supplies uniqueness and no attempt policy of its own.
//
// One attempt: the pass is idempotent — a line already taken back is left alone
// — and its retry is its own next tick, five minutes away.
func (NoticeOvertakeArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       river.QueueDefault,
		MaxAttempts: 1,
		UniqueOpts:  river.UniqueOpts{ByState: activeSweepStates},
	}
}

type noticeOvertakeWorker struct {
	approvals *approvals.Service
	notices   *notices.Store
	identity  *identity.Service
	log       *slog.Logger
}

// newNoticeOvertakeWorker builds the pass over the installation's handle. One
// handle for both modules: they are read and written in the same pass, and two
// would be two resolutions of one workspace.
func newNoticeOvertakeWorker(db *database.DB, ident *identity.Service, log *slog.Logger) *noticeOvertakeWorker {
	return &noticeOvertakeWorker{
		approvals: approvals.NewService(db),
		notices:   notices.NewStore(db),
		identity:  ident,
		log:       log,
	}
}

// Work is what covers the routes that announce nothing. Supersession,
// withdrawal and erasure all write a terminal approval and emit no event, by
// explicit design, so a consumer cannot see them and only a pass over the rows
// can take their notices back.
func (w *noticeOvertakeWorker) Work(ctx context.Context, _ *river.Job[NoticeOvertakeArgs]) error {
	passCtx, err := installationJobCtx(ctx, w.identity)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	// The pass is the actor. Every line it takes back writes an audit row, and
	// no colleague asked for any of this — the deciding one, where there was
	// one, is named on the line itself rather than on the act of clearing it.
	passCtx = principal.SystemActing(passCtx, approvals.OvertakenSweepActor)
	standing, err := w.notices.StandingApprovalReferences(passCtx)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	overtaking, err := w.overtakingAmong(passCtx, standing)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	taken, err := w.notices.OvertakeApprovalNotices(passCtx, overtaking)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if taken > 0 {
		// Worth a line: a run of these is the shape of decisions being taken out
		// from under colleagues who were asked, not of a queue draining.
		w.logger().InfoContext(ctx, "notice overtaken sweep: lines taken back",
			"lines", taken, "approvals", len(overtaking), "standing", len(standing))
	}
	return nil
}

// overtakingAmong asks approvals which of these references stopped being
// decidable, a chunk of them per statement and every one of them per pass.
func (w *noticeOvertakeWorker) overtakingAmong(
	ctx context.Context, standing []ids.ApprovalID,
) ([]notices.Overtaking, error) {
	var overtaking []notices.Overtaking
	for start := 0; start < len(standing); start += noticeOvertakeChunk {
		terminal, err := w.approvals.TerminalAmong(ctx, standing[start:min(start+noticeOvertakeChunk, len(standing))])
		if err != nil {
			return nil, err
		}
		for _, stopped := range terminal {
			overtaking = append(overtaking, notices.Overtaking{Approval: stopped.ID, By: stopped.DecidedBy})
		}
	}
	return overtaking, nil
}

func (w *noticeOvertakeWorker) logger() *slog.Logger {
	if w.log == nil {
		return slog.Default()
	}
	return w.log
}
