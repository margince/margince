// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// The event-triggered starter workflows (features/03 §5.3): small,
// deterministic automations shipped as ordinary handlers — the worked
// examples the `crm gen workflow` scaffold copies, not privileged
// built-ins. Every handler here rides the bus (a Trigger.EventType); the
// clock-triggered siblings live in handlers_clock.go. The four are
// stage_change_create_task and route_lead (task minters),
// stage_change_notify (the owner notification), and post_meeting_recap
// (the draft_email starter).

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/leadsource"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// StarterWorkflows returns the shipped handler set over the injected
// executor seams. assign_lead_owner is NOT here: its engine is
// lead-store SQL, so the contacts module provides that handler (under
// its own honest name — AUTO-NOTE-2, §3.5) and compose registers it
// beside these. ex is the seam bundle every handler's Apply threads
// into ApplyActions, whether or not this particular starter's own
// effect ever exercises every seam in it.
func StarterWorkflows(ex Executors) []workflow.Handler {
	return append([]workflow.Handler{
		stageChangeCreateTask{ex: ex},
		stageChangeNotify{ex: ex},
		postMeetingRecap{ex: ex},
		noActivityReminder{ex: ex},
		checkInCadence{ex: ex},
		renewalReminder{ex: ex},
		routeLeadCreateTask{ex: ex},
		websiteInquiry{ex: ex},
	}, listRuleWorkflows(ex)...)
}

// stageChangeCreateTaskName is the catalog key Task 6 seeds this
// starter under — CatalogEntry.Key must equal the backing handler's
// Spec().Name (automations_catalog.go's CatalogEntry doc).
const stageChangeCreateTaskName = "stage_change_create_task"

// stageChangeCreateTask keeps momentum: every OPEN stage move mints a
// follow-up task on the deal's own timeline (a close ends the cadence).
type stageChangeCreateTask struct {
	ex Executors
}

// dealStageStatusField is the deal.stage_changed payload key carrying the
// destination stage's semantic class, and dealStatusOpen is the one value
// that keeps stage_change_create_task's follow-up cadence going. deals
// emits the class under "to_status" (deals/deal_advance.go's Emit: open |
// won | lost) — NOT "to_semantic", which no emitter ever sets. Both are
// named once here and reused by the preview's equivalent predicate
// (automations_preview.go's dealPreviewDefs) so the runtime filter and its
// dry-run can never again drift onto a different field or value.
const (
	dealStageStatusField = "to_status"
	dealStatusOpen       = "open"
)

func (stageChangeCreateTask) Spec() workflow.Spec {
	return workflow.Spec{
		Name:    stageChangeCreateTaskName,
		Trigger: workflow.Trigger{EventType: eventDealStageChanged},
		Tier:    mcp.TierAutoExecute,
		// One claimed create_task and nothing else: applyCreate takes the effect
		// claim before writing, so a second pass folds instead of minting a
		// duplicate reminder.
		RedrivableWithoutDuplicating: true,
	}
}

func (w stageChangeCreateTask) Match(ctx context.Context, ev workflow.Event) (bool, error) {
	// A close (won/lost) ends the cadence; only open moves need a next
	// step. An absent payload (defensive — every real deal.stage_changed
	// carries one) is treated as an open move rather than silently dropped.
	if len(ev.Payload) == 0 {
		return true, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return false, err
	}
	status, _ := payload[dealStageStatusField].(string)
	if status != "" && status != dealStatusOpen {
		return false, nil
	}
	return stageScopeMatches(ctx, w.ex, ev)
}

// fieldKind is the "kind" json key shared by the action snapshot and the
// task effects handlers plan — named once so the string doesn't drift.
const fieldKind = "kind"

func (w stageChangeCreateTask) Plan(ctx context.Context, ev workflow.Event) (workflow.Effect, error) {
	dueAt, err := taskDueAt(ev.Params, ev.OccurredAt, 2)
	if err != nil {
		return workflow.Effect{}, err
	}
	// The fired entity IS the deal (this handler triggers only on
	// deal.stage_changed), so taskCreateEffect's string(ev.Entity.Type)
	// resolves to "deal" — the same value this map hardcoded before the
	// shared builder existed. The deal's owner is who the next step belongs
	// to; without them the task reaches nobody's own queue.
	var configured stageChangeParams
	if len(ev.Params) > 0 {
		if err := json.Unmarshal(ev.Params, &configured); err != nil {
			return workflow.Effect{}, err
		}
	}
	if configured.TaskTitle != "" {
		return ownedTaskEffectNoKey(ctx, w.ex, ev, configured.TaskTitle, dueAt)
	}
	ctx = systemtext.WithMessage(ctx, &systemtext.Message{Version: 1, Subject: systemtext.Sentence("automation.stage_change")})
	return ownedTaskEffectNoKey(ctx, w.ex, ev, systemtext.Literal("automation.stage_change"),
		dueAt)
}

func (w stageChangeCreateTask) Apply(ctx context.Context, _ workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	applied, err := ApplyActions(ctx, w.ex, eff)
	return workflow.RunResult{Applied: applied}, err
}

func (stageChangeCreateTask) IdempotencyKey(ev workflow.Event) string {
	return stageChangeCreateTaskName + ":" + ev.ID.String()
}

// routeLeadName is the catalog key Task 6 seeds this starter under
// (AUTO-NOTE-2, §3.5): "route a new lead to a task", the CREATE_TASK
// reading of those words — never confused with the OWNER-assignment
// reading of the same phrase, which lives under its own honest name,
// assign_lead_owner (contacts.LeadRoutingWorkflow), because the two are
// genuinely different acts, not two names for one automation.
const routeLeadName = "route_lead"

// defaultRouteLeadDueInDays is route_lead's own due_in_days default — a
// brand-new lead wants its first follow-up sooner than a mid-pipeline
// deal's stage-change nudge (stageChangeCreateTask's own default of 2).
const defaultRouteLeadDueInDays = 1

// routeLeadCreateTask mints a follow-up task the moment a lead is
// captured, so a brand-new lead always gets a first next-step — the
// create_task sibling of stageChangeCreateTask, over lead.created
// instead of a stage move.
type routeLeadCreateTask struct {
	ex Executors
}

func (routeLeadCreateTask) Spec() workflow.Spec {
	return workflow.Spec{
		Name:    routeLeadName,
		Trigger: workflow.Trigger{EventType: eventLeadCreated},
		Tier:    mcp.TierAutoExecute,
		// One claimed create_task and nothing else: applyCreate takes the effect
		// claim before writing, so a second pass folds instead of minting a
		// duplicate reminder.
		RedrivableWithoutDuplicating: true,
	}
}

// Match declines a lead nobody asked us for.
//
// Every lead that came from a contact — a form, a reply, a referral, a hand
// typed row — needs its first follow-up, and unlike stage_change_create_task
// there is no "wrong direction" one could have arrived from. What does not need
// one is a name the product READ off a public web page: nobody wrote in and
// nothing is owed, so minting a task says a rep owes a stranger an answer.
//
// An absent or unreadable payload fires, matching the sibling above: every real
// lead.created carries one, and the direct-create path deliberately sets no
// source system at all.
func (routeLeadCreateTask) Match(_ context.Context, ev workflow.Event) (bool, error) {
	if len(ev.Payload) == 0 {
		return true, nil
	}
	var payload crmcontracts.PublicEventLeadCreated
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return false, err
	}
	if payload.SourceSystem == nil {
		return true, nil
	}
	return !leadsource.IsPassiveDiscovery(*payload.SourceSystem), nil
}

// Plan mints the follow-up ASSIGNED to whoever answers for the lead.
//
// The owner is read off the lead through the same provider stage_change_notify
// reads its deal through, rather than imported from the contacts module: this
// module reaches no sibling, and the record is already in front of it.
//
// A lead nobody owns yet mints an unassigned task, which is the honest answer —
// it lands in the unassigned queue for somebody to pick up. Owner routing is a
// separate workflow on the same event with no ordering guarantee between them,
// so the two orders differ in WHERE the task first appears and never in whether
// it appears at all.
func (w routeLeadCreateTask) Plan(ctx context.Context, ev workflow.Event) (workflow.Effect, error) {
	dueAt, err := taskDueAt(ev.Params, ev.OccurredAt, defaultRouteLeadDueInDays)
	if err != nil {
		return workflow.Effect{}, err
	}
	ctx = systemtext.WithMessage(ctx, &systemtext.Message{Version: 1, Subject: systemtext.Sentence("automation.new_lead")})
	return ownedTaskEffectNoKey(ctx, w.ex, ev, systemtext.Literal("automation.new_lead"),
		dueAt)
}

func (w routeLeadCreateTask) Apply(ctx context.Context, _ workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	applied, err := ApplyActions(ctx, w.ex, eff)
	return workflow.RunResult{Applied: applied}, err
}

func (routeLeadCreateTask) IdempotencyKey(ev workflow.Event) string {
	return routeLeadName + ":" + ev.ID.String()
}

// dealOwnerFields is the one column stage_change_notify needs off the
// moved deal's record. Decoding straight into this local shape — rather
// than the full wire Deal type — keeps the handler from taking on a
// contract-type dependency it has no other use for.
type dealOwnerFields struct {
	OwnerID *ids.UUID `json:"owner_id"`
	// Name is what the notice calls the deal: a notice's reader gets the
	// record's own name, never an internal id as their only handle.
	Name string `json:"name"`
}

// errDealHasNoOwner is stage_change_notify's Plan decline when the moved
// deal carries no owner: notify has no honest recipient to name, and a
// nil/zero recipient would be a fabricated one, not a resolved one. This is a
// skip, not a failure — the deal legitimately has no owner and a redelivery
// would meet the same record — so the run lands 'skipped' with this reason
// (visible history, no phantom recipient) rather than a 'failed' that would
// leave the event re-delivering forever and poison its sibling handlers.
var errDealHasNoOwner = workflow.Declined("the moved deal has no assigned owner to notify")

// stageChangeNotifyName is the catalog key Task 6 seeds this starter
// under — CatalogEntry.Key must equal the backing handler's Spec().Name
// (automations_catalog.go's CatalogEntry doc).
const stageChangeNotifyName = "stage_change_notify"

// stageChangeNotify tells the owner about moves made by others.
// Their own moves remain in deal history, without becoming inbox work.
type stageChangeNotify struct {
	ex Executors
}

func (stageChangeNotify) Spec() workflow.Spec {
	return workflow.Spec{
		Name:    stageChangeNotifyName,
		Trigger: workflow.Trigger{EventType: eventDealStageChanged},
		Tier:    mcp.TierAutoExecute,
	}
}

// Both open and closed destinations can matter. Plan resolves the owner
// before deciding whether the original actor has anything new to tell them.
func (w stageChangeNotify) Match(ctx context.Context, ev workflow.Event) (bool, error) {
	return stageScopeMatches(ctx, w.ex, ev)
}

func (w stageChangeNotify) Plan(ctx context.Context, ev workflow.Event) (workflow.Effect, error) {
	rec, err := w.ex.Provider.Read(ctx, ev.Entity)
	if err != nil {
		return workflow.Effect{}, fmt.Errorf("automation: reading the moved deal: %w", err)
	}
	var deal dealOwnerFields
	if err := json.Unmarshal(rec.Fields, &deal); err != nil {
		return workflow.Effect{}, fmt.Errorf("automation: decoding the deal's owner: %w", err)
	}
	if deal.OwnerID == nil {
		return workflow.Effect{}, errDealHasNoOwner
	}
	// Retries can carry older bare UUID actor IDs as well as canonical human IDs.
	ownerID := deal.OwnerID.String()
	if ev.Actor.Type == string(principal.PrincipalHuman) &&
		(strings.EqualFold(ev.Actor.ID, ownerID) || strings.EqualFold(ev.Actor.ID, principal.HumanIDPrefix+ownerID)) {
		return workflow.Effect{}, workflow.Declined("the owner made this stage change")
	}
	metadata := &systemtext.Message{Body: systemtext.Sentence("automation.stage_unavailable")}
	dealName := deal.Name
	if dealName == "" {
		dealName = systemtext.Literal("automation.unnamed_deal")
		metadata.Subject = systemtext.Sentence("automation.unnamed_deal")
	}
	body := systemtext.Literal("automation.stage_unavailable")
	var change crmcontracts.PublicEventDealStageChanged
	if len(ev.Payload) > 0 {
		if err := json.Unmarshal(ev.Payload, &change); err != nil {
			return workflow.Effect{}, fmt.Errorf("automation: decoding the stage change: %w", err)
		}
		if change.FromStageName != nil && change.ToStageName != nil {
			body = fmt.Sprintf("%s → %s", *change.FromStageName, *change.ToStageName)
			metadata.Body = nil
		}
	}
	origin := noticeOrigin(ev)
	if origin != nil {
		origin.StageChange = &struct {
			FromName *string `json:"from_name,omitempty"`
			ToName   *string `json:"to_name,omitempty"`
		}{FromName: change.FromStageName, ToName: change.ToStageName}
	}
	args, err := json.Marshal(notifyArgs{
		Recipient: *deal.OwnerID,
		Origin:    origin,
		DedupeKey: w.IdempotencyKey(ev),
		Subject:   dealName,
		Body:      body,
	})
	if err != nil {
		return workflow.Effect{}, fmt.Errorf("automation: encoding the notify action: %w", err)
	}
	return workflow.Effect{Actions: []workflow.Action{{
		Kind: workflow.ActionNotify, Target: ev.Entity, Args: args, SystemText: metadata,
	}}}, nil
}

func (w stageChangeNotify) Apply(ctx context.Context, _ workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	applied, err := ApplyActions(ctx, w.ex, eff)
	return workflow.RunResult{Applied: applied}, err
}

func (stageChangeNotify) IdempotencyKey(ev workflow.Event) string {
	return stageChangeNotifyName + ":" + ev.ID.String()
}
