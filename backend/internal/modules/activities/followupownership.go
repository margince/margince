// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A system follow-up belongs to whoever owns the record it was minted for.
//
// Two workflows answer one lead.created with no ordering between them: one
// routes the lead to a rep, the other mints its follow-up task. When the mint
// runs first the task is written before there is an owner to write, and it
// would stay unowned forever — the lead has a rep, their follow-up does not,
// and it waits in the unassigned queue nobody opened.
//
// So the task follows the record. This reconciles from the lead's CURRENT
// owner rather than applying a change described in an event, which is what
// makes it safe to run twice, out of order, or after a reassignment: whatever
// the lead says now is what its open system tasks say when this finishes.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// OwnershipWorkflows returns the system handler that keeps a system-minted
// follow-up assigned to whoever owns the lead it belongs to. compose registers
// it beside FollowUpWorkflows.
func OwnershipWorkflows(store *Store) []workflow.Handler {
	return []workflow.Handler{followUpOwnerReconcile{store: store}}
}

// followUpOwnerReconcile answers lead.updated by making the lead's open system
// tasks agree with its owner.
type followUpOwnerReconcile struct{ store *Store }

func (followUpOwnerReconcile) Spec() workflow.Spec {
	return workflow.Spec{
		Name:    "follow_up_owner_reconcile",
		Trigger: workflow.Trigger{EventType: "lead.updated"},
		Tier:    mcp.TierAutoExecute,
	}
}

// Match fires on every lead.updated. The event does not say which column
// moved, and testing the payload for an owner change would miss the case this
// exists for: the mint losing the race arrives as an update carrying no owner
// change at all, because the owner was already there.
func (followUpOwnerReconcile) Match(_ context.Context, _ workflow.Event) (bool, error) {
	return true, nil
}

func (followUpOwnerReconcile) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	// Which tasks, and to whom, is Apply's query — the envelope carries no
	// links, so Plan states the invariant the way the auto-resolve sibling's
	// Plan does.
	args, err := json.Marshal(map[string]any{"assignee_id": "the lead's owner"})
	if err != nil {
		return workflow.Effect{}, fmt.Errorf("activities: encoding the reassignment effect: %w", err)
	}
	return workflow.Effect{Actions: []workflow.Action{{
		Kind: workflow.ActionUpdateRecord, Target: ev.Entity, Args: args,
	}}}, nil
}

func (w followUpOwnerReconcile) Apply(
	ctx context.Context, ev workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken,
) (workflow.RunResult, error) {
	moved, err := w.store.AlignSystemTaskAssigneeToLead(ctx, ids.From[ids.LeadKind](ev.Entity.ID))
	if err != nil {
		return workflow.RunResult{}, fmt.Errorf("aligning follow-ups on lead %s: %w", ev.Entity.ID, err)
	}
	if moved == 0 {
		return workflow.RunResult{}, nil
	}
	return workflow.RunResult{Applied: eff.Actions}, nil
}

func (followUpOwnerReconcile) IdempotencyKey(ev workflow.Event) string {
	// The event id, not the lead: a reassignment an hour later is a different
	// firing that must run again, and keying on the lead would swallow it.
	return "follow_up_owner_reconcile:" + ev.ID.String()
}

// AlignSystemTaskAssigneeToLead points every open system-minted task on the
// lead at the lead's current owner, and answers how many it moved.
//
// Reconciliation, not a delta: it reads the owner NOW and writes what disagrees
// with it. That is what makes the two race orders converge, and it is also what
// makes a later reassignment carry its tasks along — the same call answers both
// without knowing which one it is handling.
//
// A lead with no owner leaves its tasks alone rather than clearing them. Nobody
// asked for the work to be taken off a rep, and an unowned lead is ordinarily a
// lead whose routing has not run yet.
//
// "System-minted" is source AND captured_by together, the same pair
// CompleteOpenSystemTasksForLead uses and for the same reason: source rides the
// create wire verbatim and any caller can spell it, while captured_by is
// stamped from the authenticated principal. A planted source alone reaches
// nothing here.
//
// Each move goes through UpdateActivity — the module's own write path — so it
// carries the audit row and the activity.updated event a human reassignment
// carries. A bulk UPDATE would move the work and leave no history of who now
// owns it.
func (s *Store) AlignSystemTaskAssigneeToLead(ctx context.Context, leadID ids.LeadID) (int, error) {
	owner, ok, err := s.leadOwner(ctx, leadID)
	if err != nil || !ok {
		return 0, err
	}
	type misassigned struct {
		id      ids.ActivityID
		version int64
	}
	var wrong []misassigned
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id, a.version FROM activity a
			JOIN activity_link l ON l.activity_id = a.id
			WHERE l.lead_id = $1 AND a.kind = $2 AND a.source = $3
			  AND (a.captured_by = $4 OR a.captured_by LIKE $5)
			  AND a.is_done = false AND a.archived_at IS NULL
			  AND a.assignee_id IS DISTINCT FROM $6
			ORDER BY a.id`,
			leadID, string(crmcontracts.ActivityKindTask), systemSource,
			systemCapturedBy, systemCapturedByPattern, owner)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var task misassigned
			if err := rows.Scan(&task.id, &task.version); err != nil {
				return err
			}
			wrong = append(wrong, task)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	moved := 0
	for _, task := range wrong {
		reassigned, err := s.assignSystemTask(ctx, task.id, task.version, owner)
		if err != nil {
			return 0, fmt.Errorf("assigning system task %s: %w", task.id, err)
		}
		if reassigned {
			moved++
		}
	}
	return moved, nil
}

// leadOwner reads who answers for the lead, reporting whether anybody does.
//
// A direct read of the lead table rather than a call into people: a module
// never imports a sibling, and this is one column of one row. The write that
// follows is on this module's own table.
func (s *Store) leadOwner(ctx context.Context, leadID ids.LeadID) (ids.UserID, bool, error) {
	var owner *ids.UserID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT owner_id FROM lead WHERE id = $1 AND archived_at IS NULL`, leadID).Scan(&owner)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The lead went between the event and this read. Nothing to align,
			// and nothing here went wrong.
			return ids.UserID{}, false, nil
		}
		return ids.UserID{}, false, err
	}
	if owner == nil {
		return ids.UserID{}, false, nil
	}
	return *owner, true, nil
}

// assignmentAttempts bounds the re-read below, for the reason completionAttempts
// bounds its own: each attempt is a lost race with a different writer, and more
// than a couple means a row somebody is editing continuously.
const assignmentAttempts = 3

// assignSystemTask points one selected task at the owner, answering whether
// THIS call moved it.
//
// Version skew is a RE-READ, not a skip — the same conclusion completeSystemTask
// reached over the same select-then-write shape. Every update to an activity
// bumps its version, so skew here usually means somebody edited the subject or
// the due date, not that anybody expressed an opinion about who owns it.
// Skipping on that would leave the task pointing at nobody with no promise of
// another firing: lead.updated fires when the LEAD changes, and the task's own
// edits do not produce one.
//
// A task that has come to disagree with the lead about its owner is left alone.
// That is the case where somebody did decide, and re-reading the lead's owner
// over their decision is how an automation takes work back off the person it
// was just handed to.
func (s *Store) assignSystemTask(
	ctx context.Context, id ids.ActivityID, version int64, owner ids.UserID,
) (bool, error) {
	for attempt := 0; attempt < assignmentAttempts; attempt++ {
		_, err := s.UpdateActivity(ctx, id, UpdateActivityInput{
			AssigneeID: &owner,
			IfVersion:  &version,
		})
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, apperrors.ErrNotFound):
			// Either the task was archived between the selection and the write,
			// or the owner is no longer an active seat (UpdateActivity checks
			// the assignee exists). Both mean there is nothing to do here, and
			// failing would strand every other task this call selected.
			return false, nil
		case !errors.Is(err, apperrors.ErrVersionSkew):
			return false, err
		}
		current, err := s.GetActivity(ctx, id, storekit.LiveOnly)
		if err != nil {
			if errors.Is(err, apperrors.ErrNotFound) {
				return false, nil
			}
			return false, err
		}
		if current.AssigneeId != nil && ids.UUID(*current.AssigneeId) != owner.UUID {
			// Somebody assigned it to a different person while this call was in
			// flight. Their decision is newer than the one this call is
			// carrying, so it stands.
			return false, nil
		}
		if current.Version == nil {
			return false, nil
		}
		version = *current.Version
	}
	return false, nil
}
