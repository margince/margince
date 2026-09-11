// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A voice build reporting itself to the AI-activity projection.
//
// The router can only speak once a model call is over, so a task it reports is
// settled the moment it appears: a rep asks the product to learn their writing
// voice, sees nothing for as long as it takes, and then finds it already done.
// For work a person waits on, settled-only is worse than silence — it looks
// like nothing happened.
//
// A build has what a live line needs and the router does not: a durable row
// that is queued before any worker sees it, an attributable requester, a claim
// with a reclaim window that says when a worker has died, and an attempt that
// survives a deferral. So it carries its own occurrence and railowner.go hands
// the task here.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// VoiceBuildActivitySource names this carrier to the projection. It is
// IDENTITY, not display: two sources must never collide on one occurrence key,
// and the display kind is a separate string that may be re-pointed without
// re-keying every row.
const VoiceBuildActivitySource = "voice_build"

// VoiceBuildAITask is the api/ai-tasks.yaml task a build runs. Exported so a
// root-package fitness test can hold it to the generated task set — a module
// may not import the rail registry's own neighbours to assert that about
// itself.
const VoiceBuildAITask = string(TaskVoiceBuild)

// voiceBuildQueuedLease is how long a queued build stays believable before the
// projection reports it stalled.
//
// A queued row is waiting for a worker, not held by one, so it carries no
// reclaim window of its own — and a queue nobody drains must not render as live
// forever. Generous against the job runner's own backlog and short enough that
// a runner that is not running shows as stalled within the hour.
const voiceBuildQueuedLease = 30 * time.Minute

// activityStateDegraded is the projection's word for work that settled short of
// what it was asked for. The rail's fault arm is bounded on it, so it claims
// somebody must be TOLD.
const activityStateDegraded = "degraded"

// The build's own statuses in the projection's vocabulary.
//
// `succeeded` is this module's word for the projection's `done`; the other
// three live states carry across unchanged.
//
// deferred settles rather than staying live, and reports `degraded`. The build
// is not being worked — the workspace ran out of AI credit and the window may
// reopen hours later — and an orb lit that whole time would report work that is
// not happening. The next claim reopens the occurrence under a new attempt,
// which is exactly the reopening the projection's guard admits.
func voiceBuildActivityState(status string) string {
	switch status {
	case voiceBuildStatusSucceeded:
		return "done"
	case voiceBuildStatusDeferred:
		return activityStateDegraded
	default:
		return status
	}
}

// emitVoiceBuildActivity publishes one build's current state against an
// existing ledger row.
//
// Derived ENTIRELY from the row, so no call site can announce a state it did
// not just commit — including the attempt, which is the row's own column rather
// than anything this function counts.
//
// lease is how long a LIVE state stays believable and is ignored for a settled
// one: a closed occurrence is not claiming to work, so it has nothing to go
// stale. It travels from the caller because only the claim knows its own
// reclaim window; a queued row takes voiceBuildQueuedLease.
func emitVoiceBuildActivity(
	ctx context.Context, tx pgx.Tx, ledgerID ids.UUID, build VoiceBuild, lease time.Duration,
) error {
	task := VoiceBuildAITask
	payload := crmcontracts.InternalEventAiTaskStateChanged{
		Source: VoiceBuildActivitySource,
		// The build's own row id. A voice is learned many times over its life
		// and each build is its own occurrence, so the key is the build —
		// never the profile.
		OccurrenceKey: build.ID.String(),
		Kind:          VoiceBuildAITask,
		AiTask:        &task,
		Attempt:       build.Attempt,
		State:         voiceBuildActivityState(build.Status),
		// The instant THIS attempt became current, not the build's creation: a
		// live row ages from here, and a build deferred for a budget window
		// would otherwise be past its lease the moment it resumed.
		QueuedAt:   build.AttemptAt,
		StartedAt:  build.StartedAt,
		FinishedAt: voiceBuildSettledAt(build),
	}
	if lease > 0 && payload.FinishedAt == nil {
		seconds := int(lease.Seconds())
		payload.LeaseSeconds = &seconds
	}
	// status_detail is this module's own sentence on the failure and deferral
	// paths — never a provider's message, which FailBuild's contract already
	// requires — so it is safe to show a rep.
	if reason := voiceBuildDegradeReason(build); reason != nil {
		payload.DegradeReason = reason
	}
	// NO SUBJECT, deliberately. A build is about the reader's OWN writing
	// voice: naming a subject would put the requester's own profile on their
	// rail as though it were a record they had been working on, and there is
	// no other party to name.
	if err := storekit.EmitPipelinePayload(ctx, tx, ledgerID, payload); err != nil {
		return fmt.Errorf("publish voice build activity: %w", err)
	}
	return nil
}

// voiceBuildSettledAt is when this attempt stopped being live.
//
// completed_at is stamped by the terminal writes only; a deferral leaves it
// NULL because the build is not over, yet the ATTEMPT is — and the projection
// requires a settled state to say when — so the deferral's own write instant
// stands in.
func voiceBuildSettledAt(build VoiceBuild) *time.Time {
	if build.CompletedAt != nil {
		return build.CompletedAt
	}
	if build.Status != voiceBuildStatusDeferred {
		return nil
	}
	return build.UpdatedAt
}

// voiceBuildDegradeReason is the one sentence a settled-short build carries,
// or nil when it went through.
func voiceBuildDegradeReason(build VoiceBuild) *string {
	switch build.Status {
	case voiceBuildStatusFailed, voiceBuildStatusDeferred:
		return build.StatusDetail
	default:
		return nil
	}
}
