// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The system closes the loops it opens: an automation-minted follow-up
// task ("Follow up with the new lead") stays open forever unless somebody
// remembers to tick a box the system created — so the system watches for
// the loop actually closing and completes its own tasks. Registered as
// SYSTEM workflows (always on, never a pausable user automation), the
// same shape as people's lead-score recompute, and living here because
// the tasks are activity rows and completing one is this module's write.
//
// Only SYSTEM tasks (activity.source = 'system') are ever completed.
// Completing a human's own task is claiming work happened that they may
// not consider done; theirs stay theirs.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// taskSourceSystem is activity.source as the automation engine's create
// executor stamps it (automation's systemSource — spelled again here
// because a module cannot import a sibling's constant). Never trusted
// alone, and deliberately NOT reserved at the create wire: the engine's
// own creates flow through the same mapper as a client's
// (LogActivityInputFrom), so a value-level refusal there would refuse the
// engine itself. A caller can spell it — every selector below therefore
// pairs it with systemCapturedBy, which no client can write, and that
// pairing is the security boundary.
const taskSourceSystem = "system"

// systemCapturedBy is captured_by as the workflow engine's runs stamp it:
// storekit.CapturedBy writes the authenticated principal's ID, and the
// engine binds the system principal with ID "system" (automation's
// HandleEvent). captured_by never comes from a request body, so it is the
// unforgeable half of the "the system minted this task" predicate.
const systemCapturedBy = "system"

// FollowUpWorkflows returns the system handlers that complete open system
// follow-up tasks when the follow-up demonstrably happened: a real
// activity lands on the lead, or the lead leaves the open pool (promoted
// or disqualified — either way there is nothing left to follow up).
// compose registers them via RegisterSystemWorkflow beside the lead-score
// recompute.
func FollowUpWorkflows(store *Store) []workflow.Handler {
	return []workflow.Handler{
		followUpAutoResolve{store: store, name: "follow_up_auto_resolve", trigger: "activity.captured", leadsFromLinks: true},
		followUpAutoResolve{store: store, name: "follow_up_auto_resolve_on_promoted", trigger: "lead.promoted"},
		followUpAutoResolve{store: store, name: "follow_up_auto_resolve_on_disqualified", trigger: "lead.disqualified"},
	}
}

// followUpAutoResolve is one trigger's arm of the auto-resolve invariant.
// leadsFromLinks says the fired entity is an ACTIVITY whose linked leads
// are the ones to resolve; false means the fired entity IS the lead.
type followUpAutoResolve struct {
	store          *Store
	name           string
	trigger        string
	leadsFromLinks bool
}

func (w followUpAutoResolve) Spec() workflow.Spec {
	return workflow.Spec{
		Name:    w.name,
		Trigger: workflow.Trigger{EventType: w.trigger},
		Tier:    mcp.TierAutoExecute,
	}
}

// Match declines captured TASKS: the follow-up task the automation just
// minted arrives as its own activity.captured, and counting it as "the
// follow-up happened" would complete every reminder the moment it was
// created. A human capturing a task is a plan, not contact, and declines
// for the same reason. Every other captured kind — a call, an email, a
// meeting, a note — is the touch the reminder was waiting for. Lead
// triggers always match: leaving the open pool resolves unconditionally.
func (w followUpAutoResolve) Match(_ context.Context, ev workflow.Event) (bool, error) {
	if !w.leadsFromLinks {
		return true, nil
	}
	var captured crmcontracts.PublicEventActivityCaptured
	if len(ev.Payload) > 0 {
		if err := json.Unmarshal(ev.Payload, &captured); err != nil {
			return false, fmt.Errorf("activities: decoding the captured activity kind: %w", err)
		}
	}
	return captured.Kind != string(crmcontracts.ActivityKindTask), nil
}

func (w followUpAutoResolve) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	// The concrete task ids are Apply's query — the envelope does not
	// carry links, so Plan states the invariant (system follow-ups on
	// this entity's leads complete) the same way the lead-score
	// recompute's Plan does.
	args, err := json.Marshal(map[string]any{"is_done": true})
	if err != nil {
		return workflow.Effect{}, fmt.Errorf("activities: encoding the auto-resolve effect: %w", err)
	}
	return workflow.Effect{Actions: []workflow.Action{{
		Kind: workflow.ActionUpdateRecord, Target: ev.Entity, Args: args,
	}}}, nil
}

func (w followUpAutoResolve) Apply(ctx context.Context, ev workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	leads := []ids.LeadID{ids.From[ids.LeadKind](ev.Entity.ID)}
	if w.leadsFromLinks {
		var err error
		leads, err = w.linkedLeads(ctx, ids.From[ids.ActivityKind](ev.Entity.ID))
		if err != nil {
			return workflow.RunResult{}, err
		}
	}
	completed := 0
	for _, leadID := range leads {
		done, err := w.store.CompleteOpenSystemTasksForLead(ctx, leadID)
		if err != nil {
			return workflow.RunResult{}, fmt.Errorf("resolving follow-ups on lead %s: %w", leadID, err)
		}
		completed += done
	}
	if completed == 0 {
		return workflow.RunResult{}, nil
	}
	return workflow.RunResult{Applied: eff.Actions}, nil
}

func (w followUpAutoResolve) IdempotencyKey(ev workflow.Event) string {
	return w.name + ":" + ev.ID.String()
}

// linkedLeads answers which leads the captured activity touches — usually
// none, and then the firing is a cheap no-op. The lead-score recompute in
// people spells the same query over the same table; it is not shared
// because a module cannot import a sibling, and the table (activity_link)
// is this module's own — this side is the owner's copy.
func (w followUpAutoResolve) linkedLeads(ctx context.Context, activityID ids.ActivityID) ([]ids.LeadID, error) {
	var leads []ids.LeadID
	err := w.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT lead_id FROM activity_link WHERE activity_id = $1 AND lead_id IS NOT NULL`, activityID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.LeadID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			leads = append(leads, id)
		}
		return rows.Err()
	})
	return leads, err
}

// CompleteOpenSystemTasksForLead completes every open system-minted task
// linked to the lead, each through UpdateActivity — the module's own
// write path — so every completion carries the write shape (audit row,
// activity.updated event) exactly like a human ticking the box. A bulk
// UPDATE would be invisible history. Replays are harmless: a completed
// task no longer matches the open filter.
//
// "System-minted" is decided by source AND captured_by together: source
// rides the client create wire (and is merely refused there, not
// impossible), while captured_by is stamped from the authenticated
// principal — a planted source alone hands nothing to this path. It
// answers a COUNT rather than rows, so nothing about which records exist
// leaves a call that takes no read gate of its own; each completion's
// write is gated inside UpdateActivity.
func (s *Store) CompleteOpenSystemTasksForLead(ctx context.Context, leadID ids.LeadID) (int, error) {
	var open []ids.ActivityID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id FROM activity a
			JOIN activity_link l ON l.activity_id = a.id
			WHERE l.lead_id = $1 AND a.kind = $2 AND a.source = $3
			  AND a.captured_by = $4
			  AND a.is_done = false AND a.archived_at IS NULL
			ORDER BY a.id`, leadID, string(crmcontracts.ActivityKindTask), taskSourceSystem, systemCapturedBy)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.ActivityID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			open = append(open, id)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	done := true
	for _, id := range open {
		if _, err := s.UpdateActivity(ctx, id, UpdateActivityInput{IsDone: &done}); err != nil {
			return 0, fmt.Errorf("completing system task %s: %w", id, err)
		}
	}
	return len(open), nil
}
