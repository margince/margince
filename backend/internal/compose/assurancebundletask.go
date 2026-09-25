// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The TASK a night's findings hang from: minting one, adopting the one an
// earlier cycle left open, and holding it while the findings are filed.
//
// Split from the pass that groups the findings, which asks what to file; this
// asks what to file it ONTO, and that question is the whole of why the sweep
// touches `activity` at all — the assurance module owns none.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bundleTaskSubject is what a rep reads on their list.
//
// The count and nothing else, because the findings themselves render on the
// deal — three surfaces already show them, and a body listing them here would
// go stale the moment one of them cleared. The deal is named by the task's LINK
// rather than in this string: the list renders the record beside the subject, so
// putting the name in both spells it twice, and the copy in the text is the one
// nothing keeps current.
func bundleTaskSubject(findings int) string {
	if findings == 1 {
		return "1 forecast input needs attention"
	}
	return fmt.Sprintf("%d forecast inputs need attention", findings)
}

// taskStillOpenTx holds the task for the rest of the caller's transaction and
// reports whether it is still one a rep would look at.
//
// FOR SHARE rather than FOR UPDATE: this reads the row and files beside it, and
// a second bundling pass for another subject must not be made to wait. What it
// must block is the UPDATE that completes or archives the task, which conflicts
// with a share lock exactly as CloseCycle's does with the cycle's.
func taskStillOpenTx(ctx context.Context, tx pgx.Tx, task ids.UUID) (bool, error) {
	var open bool
	err := tx.QueryRow(ctx, `
		SELECT NOT a.is_done
		  FROM activity a
		 WHERE a.id = $1 AND a.archived_at IS NULL
		 FOR SHARE`, task).Scan(&open)
	if errors.Is(err, pgx.ErrNoRows) {
		// Archived or erased since the read. Gone is gone, as it is above.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("compose: holding the subject's task: %w", err)
	}
	return open, nil
}

// bundleTaskSource names this writer in the activity's natural key.
var bundleTaskSource = "assurance"

// mintBundleTask writes the task a subject's findings hang from.
//
// The assignee is the exception's own owner — the deal's owner as the scan saw
// it — and NOT the actor, which is the system principal this pass runs as.
// activities.taskAssignee leaves the column NULL for a system principal that
// names nobody, and an unassigned remediation task is one that reaches the
// unassigned queue rather than the contact whose deal it is about.
func mintBundleTask(
	ctx context.Context, deps bundleDeps, runID ids.UUID, subject subjectFindings,
) (ids.UUID, error) {
	line := bundleTaskSubject(len(subject.findings))
	// The natural key that makes a River retry a no-op. It is the RUN, not the
	// cycle: a retry that re-scans opens a fresh cycle with a new id, so a
	// cycle-keyed name would differ on exactly the attempt it exists to
	// deduplicate. activity.replayedActivity resolves this pair before
	// inserting, against uq_activity_source.
	sourceID := subject.kind + ":" + subject.id.String() + ":" + runID.String()
	in := activities.LogActivityInput{
		Kind:         activityKindTask,
		Subject:      &line,
		Source:       systemActor,
		SourceSystem: &bundleTaskSource,
		SourceID:     &sourceID,
		// system_remediation, and the whole feature depends on it.
		//
		// Every recency reading folds the newest activity into last_activity_at,
		// and this task is filed AGAINST the deal it is about. Left as the
		// default `human`, the system asking a question about a silent deal
		// would make that deal look freshly touched — so buyer_silent would stop
		// firing, CloseCleared would close its own finding as
		// condition_cleared, and the engine would switch itself off one deal at
		// a time with nothing failing. Migration 1788386600 carved this origin
		// out of all four recency helpers FOR this writer, before the writer
		// existed.
		Origin: activities.OriginSystemRemediation,
		Links: []activities.ActivityLinkInput{
			{EntityType: subject.kind, EntityID: subject.id},
		},
	}
	// The owner of the FIRST finding, which the read sorted to the front by
	// severity and then by money at stake. A subject's findings can carry
	// different owners only if the deal changed hands mid-pass; taking the most
	// material one is the answer that does not depend on map order.
	if owner := subject.findings[0].OwnerID; owner != nil {
		assignee := ids.From[ids.UserKind](*owner)
		in.AssigneeID = &assignee
	}

	task, err := writeBundleTask(ctx, deps, in)
	if err == nil {
		return task, nil
	}

	// A SEAT THAT CANNOT HOLD WORK COSTS THE TASK ITS ASSIGNEE, NOT ITS
	// EXISTENCE.
	//
	// owner_id is a snapshot the scan took, and nothing revalidates it: a rep
	// who has left the company still owns their open deals, and
	// ensureAssigneeCanHoldWork refuses a seat that is deactivated, archived or
	// an agent. Losing the task there would mean the deals of everyone who left
	// silently stop being checked — the one population most likely to be
	// carrying findings nobody is watching. Unassigned is where taskAssignee
	// itself puts work nobody can be given, so this lands in the same queue.
	if in.AssigneeID == nil || !unassignableSeat(err) {
		return ids.UUID{}, fmt.Errorf("compose: minting the task: %w", err)
	}
	in.AssigneeID = nil
	task, err = writeBundleTask(ctx, deps, in)
	if err != nil {
		return ids.UUID{}, fmt.Errorf("compose: minting the task unassigned: %w", err)
	}
	return task, nil
}

// unassignableSeat reports whether the refusal was about WHO the task was aimed
// at rather than about the task.
//
// The two shapes ensureAssigneeCanHoldWork answers with, and only those: a seat
// that is missing, inactive or archived is ErrNotFound — deliberately
// indistinguishable from a guessed id — and an agent seat is AgentAssigneeError.
// Anything else is a real failure and must not be retried into a task nobody is
// asked to do.
func unassignableSeat(err error) bool {
	var agent *activities.AgentAssigneeError
	return errors.Is(err, apperrors.ErrNotFound) || errors.As(err, &agent)
}

// writeBundleTask mints the activity in the bundling store's transaction.
func writeBundleTask(
	ctx context.Context, deps bundleDeps, in activities.LogActivityInput,
) (ids.UUID, error) {
	var task ids.UUID
	err := deps.bundles.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		created, _, err := deps.activities.LogActivityTx(ctx, tx, in)
		if err != nil {
			return err
		}
		task = ids.UUID(created.Id)
		return nil
	})
	return task, err
}

// adoptOpenTask finds a still-open task an earlier cycle raised for this
// subject, and settles the extras when several are open at once.
//
// TWO JOBS, and they are one function because the second is how an
// installation that already accumulated duplicates gets back to one row. The
// cycle-scoped lookup this stands behind could only ever see one night, so a
// finding that stayed true for a week left a week of identical tasks: same
// subject line, no body, nothing to tell them apart. Newest survives — it is
// the one a rep has most likely already seen — and the rest are archived
// through the activities door, which writes their audit and outbox rows.
//
// A task somebody has DONE is not adopted and not archived. The finding being
// true again after a rep answered it is a new question, honestly asked.
func adoptOpenTask(
	ctx context.Context, deps bundleDeps, subject subjectFindings,
) (ids.UUID, bool, error) {
	known, err := deps.bundles.TaskIDsForSubject(ctx, subject.kind, subject.id)
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("compose: reading the subject's earlier tasks: %w", err)
	}
	// The id AND the version it was read at. The read and the archive below are
	// separate statements, and a rep can complete or archive a task between
	// them: without the version the sweep would archive a completion it never
	// saw, which HIDES the rep's answer rather than losing work. Pinning it
	// makes the write refuse instead — the door's own ifVersion, which is the
	// concurrency primitive every other writer in this tree uses.
	type openTask struct {
		id      ids.UUID
		version *int64
	}
	var open []openTask
	for _, id := range known {
		act, err := deps.activities.GetActivity(ctx, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
		if errors.Is(err, apperrors.ErrNotFound) {
			// Archived, erased, or swept. Gone is gone: the cycle mints.
			continue
		}
		if err != nil {
			return ids.UUID{}, false, fmt.Errorf("compose: reading an earlier task: %w", err)
		}
		// THE TASK MUST STILL BE ABOUT THIS SUBJECT. assurance_task_item records
		// where a task was FILED, and a relink moves where it lives: a task
		// raised for deal A and since relinked to deal B is B's now, and
		// adopting it would hang A's findings off B's row — or archive a task
		// somebody moved on purpose.
		if !linksToSubject(act, subject) {
			continue
		}
		if act.IsDone != nil && *act.IsDone {
			continue
		}
		open = append(open, openTask{id: id, version: (*int64)(act.Version)})
	}
	if len(open) == 0 {
		return ids.UUID{}, false, nil
	}
	// TaskIDsForSubject answers newest first, so the head is the survivor.
	for _, extra := range open[1:] {
		// A LOST RACE IS NOT A FAULT HERE. Somebody wrote to this task between
		// the read above and this line — most often the rep completing it —
		// and their answer stands: the duplicate is left alone and the next
		// cycle reads it afresh, which is the same judgement this function
		// makes about a task that was already done when it looked. Skew is
		// somebody having edited it; not-found is somebody having archived or
		// erased it, and the settled duplicate this sweep wanted is exactly
		// what that leaves behind.
		//
		// One condition rather than an arm of its own, so a lost race and an
		// ordinary success leave this loop by the same path: the sweep settles
		// what it still may and reports nothing unusual, because nothing
		// unusual happened.
		_, err := deps.activities.ArchiveActivity(ctx, ids.From[ids.ActivityKind](extra.id), extra.version)
		if err != nil && !errors.Is(err, apperrors.ErrVersionSkew) && !errors.Is(err, apperrors.ErrNotFound) {
			return ids.UUID{}, false, fmt.Errorf("compose: settling a duplicate task: %w", err)
		}
	}
	return open[0].id, true, nil
}

// linksToSubject reports whether a task still names the subject it was raised
// for. A link is a live fact about the task; the bundling row records only
// where it was filed on the night it was minted.
func linksToSubject(act crmcontracts.Activity, subject subjectFindings) bool {
	if act.Links == nil {
		return false
	}
	for _, link := range *act.Links {
		if string(link.EntityType) == subject.kind && ids.UUID(link.EntityId) == subject.id {
			return true
		}
	}
	return false
}
