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

// systemSource is activity.source as the automation engine's create
// executor stamps it (automation's own systemSource — spelled again here
// because a module cannot import a sibling's constant). Never trusted
// alone, and deliberately NOT reserved at the create wire: the engine's
// own creates flow through the same mapper as a client's
// (LogActivityInputFrom), so a value-level refusal there would refuse the
// engine itself. A caller can spell it — so every selector over it in
// this package (the follow-up resolver below, the last-touch scan's
// genuine-engagement exclusion) pairs it with systemCapturedBy, which no
// client can write, and that PAIRING is the security boundary. The one
// pair of constants for the package, so the next selector cannot copy
// half the predicate.
const systemSource = "system"

// systemCapturedBy is captured_by as the workflow engine's runs stamp it:
// storekit.CapturedBy writes the authenticated principal's ID, and the
// engine binds the system principal with ID "system" (automation's
// HandleEvent). captured_by never comes from a request body, so it is the
// unforgeable half of the "the system wrote this row" predicate.
const systemCapturedBy = "system"

// systemCapturedByPattern is the LIKE pattern that matches every system
// principal, because captured_by carries the principal's ID and those IDs
// are NAMESPACED: the bus binds a bare "system", while every job binds its
// own "system:<job>" — "system:time-scan", "system:brief-overnight",
// "system:comms-send" and thirty-odd more.
//
// Comparing against the bare literal alone is what let a reminder task the
// TIME SCAN minted count as a genuine engagement: it carries
// captured_by "system:time-scan", the equality missed it, and the row the
// engine wrote reset the very clock the engine reads. A prefix match is a
// spelling test and would normally be refused here — it earns its place
// because the ':' namespace IS the convention every system actor is bound
// under, and because captured_by is server-stamped from the principal, so
// no caller can reach this pattern by writing one.
const systemCapturedByPattern = systemCapturedBy + ":%"

// FollowUpWorkflows returns the system handlers that complete open system
// follow-up tasks when the follow-up demonstrably happened: a real
// activity lands on the lead, or the lead leaves the open pool (promoted
// or disqualified — either way there is nothing left to follow up).
// compose registers them via RegisterSystemWorkflow beside the lead-score
// recompute.
func FollowUpWorkflows(store *Store) []workflow.Handler {
	return []workflow.Handler{
		followUpAutoResolve{store: store, name: "follow_up_auto_resolve", trigger: activityCapturedTrigger, leadsFromLinks: true},
		followUpAutoResolve{store: store, name: "follow_up_auto_resolve_on_promoted", trigger: leadPromotedTrigger},
		followUpAutoResolve{store: store, name: "follow_up_auto_resolve_on_disqualified", trigger: leadDisqualifiedTrigger},
	}
}

// The three triggers this file's Apply arms watch. Named together because
// they are read together (FollowUpWorkflows, Apply's leadPromotedTrigger
// check), and a third literal beside two names is the spelling that drifts.
const (
	activityCapturedTrigger = "activity.captured"
	// leadPromotedTrigger is the one trigger whose Apply also has to resolve
	// by PERSON rather than by lead — see the comment on Apply for why.
	leadPromotedTrigger     = "lead.promoted"
	leadDisqualifiedTrigger = "lead.disqualified"
)

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
	// A PROMOTED lead is a different shape from a disqualified one.
	// carryLeadActivities (people/promote.go) moves the follow-up task's
	// link from the lead onto the person it became, inside the SAME
	// transaction that emits this event — so the lead-keyed completion above
	// never finds it once a lead has genuinely promoted; only the payload
	// still names where the task went.
	//
	// It names the CARRIED ACTIVITIES, which is what makes both outcomes
	// answerable by one rule. Completing "every open system task on the
	// person" is exact for a freshly created person and wrong for a merge:
	// the survivor can already carry its own open system-minted reminders —
	// no_activity_reminder and check_in_cadence anchor on a person the same
	// way a lead's follow-up does — so that reading would tick off work this
	// promotion never touched, with an audit row claiming a follow-up
	// happened that did not. Completing the ids the promotion actually moved
	// is exact for both, because it is a fact about the promotion rather than
	// about the person.
	//
	// The person-keyed path stays for a payload written before the ids were
	// carried, and stays gated on "created" there for exactly the reason
	// above. A replayed old event is then no worse than it was.
	if w.trigger == leadPromotedTrigger {
		promoted, err := decodeLeadPromoted(ev.Payload)
		if err != nil {
			return workflow.RunResult{}, fmt.Errorf("decoding the promoted person: %w", err)
		}
		done, err := w.completePromoted(ctx, promoted)
		if err != nil {
			return workflow.RunResult{}, err
		}
		completed += done
	}
	if completed == 0 {
		return workflow.RunResult{}, nil
	}
	return workflow.RunResult{Applied: eff.Actions}, nil
}

// completePromoted resolves the follow-ups one promotion carried.
//
// By the carried ids when the payload names them, and by the person only when
// it does not — see the call site for why the second is gated on a freshly
// created person and the first needs no gate at all.
func (w followUpAutoResolve) completePromoted(
	ctx context.Context, promoted promotedLead,
) (int, error) {
	if len(promoted.CarriedActivityIDs) > 0 {
		done, err := w.store.CompleteCarriedSystemTasks(ctx, promoted.CarriedActivityIDs)
		if err != nil {
			return 0, fmt.Errorf("resolving the follow-ups carried onto person %s: %w",
				promoted.PersonID, err)
		}
		return done, nil
	}
	if promoted.DedupeOutcome != dedupeOutcomeCreated {
		return 0, nil
	}
	done, err := w.store.CompleteOpenSystemTasksForPerson(ctx, promoted.PersonID)
	if err != nil {
		return 0, fmt.Errorf("resolving follow-ups on person %s: %w", promoted.PersonID, err)
	}
	return done, nil
}

// dedupeOutcomeCreated is people.QualifyLead's own spelling
// (promote.go) for "a fresh person, not a merge into a survivor" — the one
// outcome where completing every open system task on the person cannot reach
// anything this promotion did not itself just carry there.
const dedupeOutcomeCreated = "created"

// promotedLead is what a lead.promoted payload says about where a lead's
// tasks went, and whether that person existed before this promotion.
type promotedLead struct {
	PersonID      ids.PersonID
	DedupeOutcome string
	// CarriedActivityIDs are the activities this promotion moved from the lead
	// onto the person. Empty on a payload written before the event carried
	// them, which is the one case the person-keyed reading still serves.
	CarriedActivityIDs []ids.UUID
}

// decodeLeadPromoted reads a lead.promoted payload — the only place, once
// carryLeadActivities has run, that still says where its tasks went.
func decodeLeadPromoted(payload json.RawMessage) (promotedLead, error) {
	var body crmcontracts.PublicEventLeadPromoted
	if err := json.Unmarshal(payload, &body); err != nil {
		return promotedLead{}, fmt.Errorf("activities: decoding lead.promoted's payload: %w", err)
	}
	out := promotedLead{
		PersonID:      ids.From[ids.PersonKind](ids.UUID(body.PromotedPersonId)),
		DedupeOutcome: body.DedupeOutcome,
	}
	if body.CarriedActivityIds != nil {
		for _, carried := range *body.CarriedActivityIds {
			out.CarriedActivityIDs = append(out.CarriedActivityIDs, ids.UUID(carried))
		}
	}
	return out, nil
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
