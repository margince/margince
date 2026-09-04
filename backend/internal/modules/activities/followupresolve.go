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
	// Only for a FRESH person (dedupe_outcome "created"). A promotion that
	// MERGES into an existing survivor carries the same task, but the
	// survivor can already carry its own open system-minted reminders —
	// no_activity_reminder and check_in_cadence anchor on a person the same
	// way a lead's follow-up does — and completing every open system task on
	// the person would tick off work this promotion has nothing to do with.
	// A newly minted person cannot yet carry anything else, so completing by
	// person id is exact there and only there. A MERGED promotion's carried
	// follow-up is left open by this arm; nothing else resolves it either.
	if w.trigger == leadPromotedTrigger {
		promoted, err := decodeLeadPromoted(ev.Payload)
		if err != nil {
			return workflow.RunResult{}, fmt.Errorf("decoding the promoted person: %w", err)
		}
		if promoted.DedupeOutcome == dedupeOutcomeCreated {
			done, err := w.store.CompleteOpenSystemTasksForPerson(ctx, promoted.PersonID)
			if err != nil {
				return workflow.RunResult{}, fmt.Errorf("resolving follow-ups on person %s: %w", promoted.PersonID, err)
			}
			completed += done
		}
	}
	if completed == 0 {
		return workflow.RunResult{}, nil
	}
	return workflow.RunResult{Applied: eff.Actions}, nil
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
}

// decodeLeadPromoted reads a lead.promoted payload — the only place, once
// carryLeadActivities has run, that still says where its tasks went.
func decodeLeadPromoted(payload json.RawMessage) (promotedLead, error) {
	var body crmcontracts.PublicEventLeadPromoted
	if err := json.Unmarshal(payload, &body); err != nil {
		return promotedLead{}, fmt.Errorf("activities: decoding lead.promoted's payload: %w", err)
	}
	return promotedLead{
		PersonID:      ids.From[ids.PersonKind](ids.UUID(body.PromotedPersonId)),
		DedupeOutcome: body.DedupeOutcome,
	}, nil
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
// linked to the lead — see completeOpenSystemTasksLinkedBy for the write
// shape, the version-skew handling and what "system-minted" means.
func (s *Store) CompleteOpenSystemTasksForLead(ctx context.Context, leadID ids.LeadID) (int, error) {
	return s.completeOpenSystemTasksLinkedBy(ctx, leadLinkColumn, leadID.UUID)
}

// CompleteOpenSystemTasksForPerson is CompleteOpenSystemTasksForLead's
// sibling for a lead that PROMOTED: carryLeadActivities (people/promote.go)
// re-points a follow-up task's link from the lead onto the person it became,
// in the same transaction that emits lead.promoted, so a lead id can no
// longer find it — only the person id it was carried to can.
//
// Completion is bounded to the tasks that existed when the person did. This
// arm's caller runs asynchronously off the outbox, so "the person is fresh
// and cannot yet carry anything else" is only true up to the moment the
// promotion committed — a sibling automation (no_activity_reminder,
// check_in_cadence) can anchor its own system task on the same person before
// this handler runs, and completing every open system task on the person
// would claim that task too.
//
// The bound is the person's OWN created_at, and it takes no argument on
// purpose. A promotion mints the person in the same transaction that carries
// the tasks onto them, so that row's creation IS the promotion instant — and
// unlike a timestamp threaded in from the caller it is written by the same
// clock as the activity.created_at it is compared against. The event's
// app-stamped OccurredAt used to fill this role, and could not: a host clock
// trailing the database's by more than the gap between a follow-up task's
// creation and the promotion put the carried task on the wrong side of the
// bound, which returns completed == 0 with no error — a loop left open
// forever and nothing to notice it by.
func (s *Store) CompleteOpenSystemTasksForPerson(ctx context.Context, personID ids.PersonID) (int, error) {
	return s.completeOpenSystemTasksLinkedBy(ctx, personLinkColumn, personID.UUID)
}

// completeOpenSystemTasksLinkedBy completes every open system-minted task the
// given column and value select, each through UpdateActivity — the module's
// own write path — so every completion carries the write shape (audit row,
// activity.updated event) exactly like a human ticking the box. A bulk UPDATE
// would be invisible history. Replays are harmless: a completed task no
// longer matches the open filter.
//
// The selection and the completions are separate transactions, so two
// firings — an activity.captured racing a lead.promoted, or two firings that
// both resolve through this same helper — can both select the same open
// task. Each completion therefore carries the version the selection read,
// and a task somebody else finished first answers version skew and is
// SKIPPED. Without it the loser writes is_done = true over a row that
// already says so, and the task's history shows two identical completions of
// the same thing.
//
// The count is what THIS call completed, which is also why skew is not an
// error: another firing completing the task is the outcome this one wanted.
//
// "System-minted" is decided by source AND captured_by together: source
// rides the client create wire verbatim (any caller can spell "system" —
// see systemSource's doc), while captured_by is stamped from the authenticated
// principal — a planted source alone hands nothing to this path. It
// answers a COUNT rather than rows, so nothing about which records exist
// leaves a call that takes no read gate of its own; each completion's
// write is gated inside UpdateActivity.
//
// `column` is unexported and passed only by the two callers above, each a
// compile-time literal ("lead_id" / "person_id") — never a value off a
// request body. Its placeholder is spelled right here, beside the argument
// that fills it, rather than split across a call boundary.
//
// The created_at bound is derived from `column` rather than asked for
// alongside it, because the two are one fact: the bound reads the PERSON row
// $1 names, so it means something only when $1 is a person id. A separate
// flag would let a caller pair it with leadLinkColumn, and that pairing does
// not fail — it looks a lead id up in `person`, finds nothing, and completes
// nothing at all. leadLinkColumn asks the original, unbounded question: a
// lead's own follow-up cannot be confused with a sibling automation's task
// the way a shared person id can.
func (s *Store) completeOpenSystemTasksLinkedBy(ctx context.Context, column string, linkValue ids.UUID) (int, error) {
	type openTask struct {
		id      ids.ActivityID
		version int64
	}
	var open []openTask
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := []any{
			linkValue, string(crmcontracts.ActivityKindTask), systemSource,
			systemCapturedBy, systemCapturedByPattern,
		}
		query := storekit.SQLf(`
			SELECT a.id, a.version FROM activity a
			JOIN activity_link l ON l.activity_id = a.id
			WHERE l.%s = $1 AND a.kind = $2 AND a.source = $3
			  AND (a.captured_by = $4 OR a.captured_by LIKE $5)
			  AND a.is_done = false AND a.archived_at IS NULL`, column)
		if column == personLinkColumn {
			// Both sides are Postgres's clock: person.created_at and
			// activity.created_at are each DEFAULT now(), and reading them
			// in one statement leaves no second clock for a caller to
			// introduce. A person that has since been deleted makes the
			// subquery NULL, so nothing matches and nothing is completed —
			// the safe direction.
			query += " AND a.created_at <= (SELECT p.created_at FROM person p WHERE p.id = $1)"
		}
		query += " ORDER BY a.id"
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var task openTask
			if err := rows.Scan(&task.id, &task.version); err != nil {
				return err
			}
			open = append(open, task)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	completed := 0
	for _, task := range open {
		flipped, err := s.completeSystemTask(ctx, task.id, task.version)
		if err != nil {
			return 0, fmt.Errorf("completing system task %s: %w", task.id, err)
		}
		if flipped {
			completed++
		}
	}
	return completed, nil
}

// The two activity_link columns this file resolves follow-ups through, named
// because completeOpenSystemTasksLinkedBy derives its created_at bound from
// which one it was handed. Both are compile-time literals reaching SQL as
// identifiers, never a value off a request body.
const (
	leadLinkColumn   = "lead_id"
	personLinkColumn = "person_id"
)

// completionAttempts bounds the re-read below. Each attempt is a lost race
// with a DIFFERENT writer, so more than a couple means a row somebody is
// editing continuously; failing then is honest, and the workflow's own retry
// is the right place for the wait.
const completionAttempts = 3

// completeSystemTask completes one selected task, answering whether THIS call
// is what flipped it.
//
// The version is the one the selection read, so a row that moved underneath
// answers skew rather than writing. What happens next depends on why it moved,
// and the two reasons are not alike:
//
//   - a sibling firing completed it — there is nothing left to do, and writing
//     anyway would put a second identical completion in the task's history;
//   - anything else touched it — the task is still open and still has to be
//     completed, because the loop that opened it has closed and no later
//     firing is promised.
//
// So skew is a re-read, not a skip. Skipping unconditionally trades a noisy
// history for a follow-up task that can stay open forever, which is the worse
// of the two.
func (s *Store) completeSystemTask(ctx context.Context, id ids.ActivityID, version int64) (bool, error) {
	done := true
	for attempt := 0; attempt < completionAttempts; attempt++ {
		_, err := s.UpdateActivity(ctx, id, UpdateActivityInput{IsDone: &done, IfVersion: &version})
		if err == nil {
			return true, nil
		}
		if errors.Is(err, apperrors.ErrNotFound) {
			// Archived between the selection and the write: the row lock is
			// taken live, so a task that has gone answers not-found. There is
			// no task to finish and nothing here went wrong — failing the
			// firing would strand every OTHER task it selected.
			return false, nil
		}
		if !errors.Is(err, apperrors.ErrVersionSkew) {
			return false, err
		}
		current, err := s.GetActivity(ctx, id, storekit.LiveOnly)
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if current.IsDone != nil && *current.IsDone {
			return false, nil
		}
		if current.Version == nil {
			return false, fmt.Errorf("task %s reports no version to retry against", id)
		}
		version = int64(*current.Version)
	}
	return false, fmt.Errorf("task %s was edited under every one of %d completion attempts", id, completionAttempts)
}
