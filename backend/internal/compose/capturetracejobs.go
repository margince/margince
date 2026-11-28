// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The trace sweep's two job kinds: a cadenced dispatcher that enumerates the
// live fleet, and a workspace child that deletes one tenant's tail.
//
// Two rather than one for the reason every fan-out here is two: a kind that
// both ticks on a clock and carries a tenant has no honest answer for whose
// data the tick touched.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/chatclarify"
	"github.com/margince/margince/backend/internal/compose/tracesweep"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// traceRetention is the window the sweep enforces, and the same one the read
// applies. They are one constant so a change cannot leave the API showing rows
// the sweep has already decided to delete, or hiding rows it still holds.
const traceRetention = capture.TraceWindowHours * time.Hour

// CaptureTraceSweepArgs runs one fleet-wide trace sweep.
type CaptureTraceSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (CaptureTraceSweepArgs) Kind() string { return "capture_trace_sweep" }

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (CaptureTraceSweepArgs) FleetWide() {
	// Intentionally empty: jobs.FleetWide is a marker interface, and the method
	// exists to be satisfied rather than called. The pass's work is
	// Work(), which enumerates the fleet.
}

type captureTraceSweepWorker struct {
	pool   *pgxpool.Pool
	purger *CapturePurger
}

// Work sweeps EVERY workspace, archived ones included — the enumeration the
// other retention passes use, and for the reason its own comment gives:
// archiving a workspace does not un-store the data inside it, and storage
// limitation does not pause because a tenant stopped logging in.
//
// It matters more here than for most passes. Under the trace_payloads posture
// these rows hold correspondence content, so skipping archived tenants would
// keep it past the 24-hour retention this feature promises, in exactly the
// workspaces nobody looks at any more.
//
// One worker where there were two (ADR-0103).
func (w *captureTraceSweepWorker) Work(ctx context.Context, _ *river.Job[CaptureTraceSweepArgs]) error {
	return jobs.FaultContext(ctx, runEveryWorkspacePass(ctx, w.pool, w.sweepWorkspace))
}

func (w *captureTraceSweepWorker) sweepWorkspace(ctx context.Context, workspace ids.UUID) error {
	return tracesweep.Workspace(ctx, w.pool, workspace, traceRetention, w.purger)
}

// ChatClarificationSweepArgs runs one fleet-wide pass over the forwards colleagues
// made to company chat accounts without naming an author.
type ChatClarificationSweepArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (ChatClarificationSweepArgs) Kind() string { return "chat_clarification_sweep" }

// FleetWide marks this as answering for the whole installation: it walks the
// workspaces itself (jobs.FleetWide).
func (ChatClarificationSweepArgs) FleetWide() {
	// Intentionally empty: jobs.FleetWide is a marker interface.
}

type chatClarificationSweepWorker struct{ sweep chatclarify.Sweep }

// Work asks the colleagues of every workspace who wrote what they forwarded.
func (w *chatClarificationSweepWorker) Work(ctx context.Context, _ *river.Job[ChatClarificationSweepArgs]) error {
	return jobs.FaultContext(ctx, runEveryWorkspacePass(ctx, w.sweep.Pool, w.sweep.Workspace))
}
