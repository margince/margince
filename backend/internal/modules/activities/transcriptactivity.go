// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A transcript reading reports itself to the AI-activity rail.
//
// The router can only speak once a model call is over, so a task it reports is
// settled the moment it appears: a rep asks for a reading, sees nothing for as
// long as it takes, and then finds it already finished. For work a person waits
// on, settled-only is worse than silence — it looks like nothing happened.
//
// This reading has what a live line needs and the router does not: a durable
// row, an attributable requester, a lease that says when a claim has died, and
// four states that are exactly the projection's live and terminal pair. So it
// carries its own occurrence, and railowner.go hands the task here.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TranscriptActivitySource names this source to the AI-activity projection. It
// is IDENTITY, not display: two sources must never collide on one occurrence
// key, and the display kind is a separate string that may be re-pointed without
// re-keying every row.
const TranscriptActivitySource = "transcript_read"

// TranscriptAITask is the api/ai-tasks.yaml task a transcript reading runs.
// Exported so a root-package fitness test can hold it to the generated task
// set — a module may not import the ai module to assert that about itself.
const TranscriptAITask = "transcript_propose"

// emitTranscriptActivity publishes one reading's current state to the
// AI-activity projection.
//
// Derived ENTIRELY from the row, so no call site can disagree with another
// about what it just wrote — including the attempt, which is the row's own
// column rather than anything this function counts.
//
// The lease travels with the event because only this package knows it. The
// projection cannot ask (it may not import this module) and must not guess: a
// reader that renders a dead reading as live is the whole failure this closes.
func emitTranscriptActivity(ctx context.Context, tx pgx.Tx, ledgerID ids.UUID, read TranscriptRead) error {
	lease := int(TranscriptReadLease.Seconds())
	task := TranscriptAITask
	payload := crmcontracts.InternalEventAiTaskStateChanged{
		Source: TranscriptActivitySource,
		// The reading's own row id. One transcript is read many times over its
		// life and each reading is its own occurrence, so the key is the
		// reading — never the meeting.
		OccurrenceKey: read.ID.String(),
		Kind:          TranscriptAITask,
		AiTask:        &task,
		Attempt:       read.Attempt,
		State:         read.Status,
		QueuedAt:      read.AttemptAt,
		StartedAt:     read.StartedAt,
		FinishedAt:    read.FinishedAt,
		LeaseSeconds:  &lease,
		SubjectType:   ptrOrNil("activity"),
		SubjectId:     contractUUID(read.ActivityID.UUID),
		// NO SUBJECT LABEL, deliberately.
		//
		// The name would be the meeting's subject, and reading it here means
		// reading the activity table on the emit path — where the audience and
		// restricted-row clauses have no caller to compose against. The worker
		// transitions run under a system principal, which reads every audience
		// away, so the label would be resolved by the one reader that cannot
		// ask the question the gates exist to force.
		//
		// The unnamed sentence is a complete one ("I'm reading the meeting
		// transcript"), so the cost is a nicety and the alternative was
		// ratifying a reader of held rows to buy it.
		// StatusDetail is this module's own vocabulary on the failure path and
		// a rep-facing sentence on the empty-but-correct one — a transcript
		// that states no next steps is a right answer, not a fault.
		Summary: read.StatusDetail,
	}
	if read.Status == TranscriptReadFailed {
		payload.DegradeReason = read.StatusDetail
		payload.Summary = nil
	}
	if err := storekit.EmitPipelinePayload(ctx, tx, ledgerID, payload); err != nil {
		return fmt.Errorf("publish transcript reading activity: %w", err)
	}
	return nil
}

// logTranscriptActivity writes the ledger row a state change needs when the
// transition has none of its own, and publishes the change against it.
//
// Every event carries a ledger trace link, and a reading's transitions write
// none: queueing, claiming and re-arming are all row updates with no audit of
// their own. The system_log row is what keeps the outcome attributable, which
// is the condition on riding the bus without an entity ref at all.
func logTranscriptActivity(ctx context.Context, tx pgx.Tx, read TranscriptRead) error {
	ledgerID, err := storekit.LogSystem(ctx, tx, "ai_task.state_changed", map[string]any{
		ledgerSourceKey: TranscriptActivitySource, "occurrence_key": read.ID.String(),
		"state": read.Status, "attempt": read.Attempt,
	})
	if err != nil {
		return fmt.Errorf("log transcript reading state change: %w", err)
	}
	return emitTranscriptActivity(ctx, tx, ledgerID, read)
}
