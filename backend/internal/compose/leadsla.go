// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The lead first-response SLA's cross-module half (formulas §18.2). The
// people module owns the clock and the breach mark; the escalation writes a
// task, which is an activity — a table people may not write — so the edge is
// injected here: a system workflow on lead.sla_breached that logs the task
// through the activities store, and the scan pass that rides the clock-
// trigger job.
//
// The RC-5 `notify_and_task` default ships both halves: the task every rep
// already works from, and — now that the durable notice transport exists
// (noticesseam.go) — the notice on the escalation target's Worklist.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// leadSLATaskSource is the (source_system) under which the escalation task
// is logged; with the lead id and breach time as source_id it makes the
// write idempotent across redelivery of the same breach event.
const leadSLATaskSource = "lead_sla"

// activityKindTask is the activity kind the escalation writes.
const activityKindTask = "task"

// leadSLAEscalation is the §18.2 escalation: on lead.sla_breached, one
// kind=task activity linked to the lead, due now, assigned to the
// escalation target the event names (the owner today), titled after the
// breach.
type leadSLAEscalation struct {
	activities *activities.Store
	notices    *notices.Store
	now        func() time.Time
}

func (leadSLAEscalation) Spec() workflow.Spec {
	return workflow.Spec{
		Name:    "lead_sla_escalation",
		Trigger: workflow.Trigger{EventType: "lead.sla_breached"},
		Tier:    mcp.TierAutoExecute,
	}
}

func (leadSLAEscalation) Match(context.Context, workflow.Event) (bool, error) { return true, nil }

func (leadSLAEscalation) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	return workflow.Effect{Actions: []workflow.Action{{
		Kind: workflow.ActionCreateTask, Target: ev.Entity,
	}}}, nil
}

func (w leadSLAEscalation) Apply(ctx context.Context, ev workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	var payload crmcontracts.PublicEventLeadSlaBreached
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return workflow.RunResult{}, fmt.Errorf("decode lead.sla_breached: %w", err)
	}
	subject := "SLA breach — first response overdue"
	sourceSystem := leadSLATaskSource
	sourceID := ev.Entity.ID.String() + ":" + payload.Deadline.UTC().Format(time.RFC3339)
	due := w.now().UTC()
	in := activities.LogActivityInput{
		Kind:         activityKindTask,
		Subject:      &subject,
		OccurredAt:   &due,
		DueAt:        &due,
		SourceSystem: &sourceSystem,
		SourceID:     &sourceID,
		Links:        []activities.ActivityLinkInput{{EntityType: flipObjectLead, EntityID: ev.Entity.ID}},
		Source:       systemActor,
	}
	if payload.EscalationTarget != nil {
		target := ids.From[ids.UserKind](ids.UUID(*payload.EscalationTarget))
		in.AssigneeID = &target
	}
	if _, _, err := w.activities.LogActivity(ctx, in); err != nil {
		return workflow.RunResult{}, fmt.Errorf("log sla escalation task: %w", err)
	}
	// The notify half RC-5 promised, now that a transport exists: the same
	// person the task escalates to gets the durable line on their Worklist.
	// A breach with no named target still writes its task; there is nobody
	// to address the notice to, and inventing one would misdeliver it.
	if payload.EscalationTarget != nil {
		target := ids.From[ids.UserKind](ids.UUID(*payload.EscalationTarget))
		if _, err := w.notices.Create(ctx, target, noticeKindLeadSLA, subject,
			"A lead's first response is overdue; its escalation task is on your list."); err != nil {
			return workflow.RunResult{}, fmt.Errorf("record sla escalation notice: %w", err)
		}
	}
	return workflow.RunResult{Applied: eff.Actions}, nil
}

func (leadSLAEscalation) IdempotencyKey(ev workflow.Event) string {
	return "lead_sla_escalation:" + ev.ID.String()
}

// scanLeadSLA is the per-workspace breach pass the clock-trigger job runs
// after the automation scan: it marks every newly breached lead and emits
// lead.sla_breached for each, which the escalation above consumes.
func scanLeadSLA(ctx context.Context, db *database.DB, now func() time.Time, log *slog.Logger) error {
	wsCtx := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "system:lead-sla-scan"})
	wsCtx = principal.WithCorrelationID(wsCtx, ids.NewV7())
	breaches, err := people.NewStore(db).ScanLeadSLA(wsCtx, now().UTC())
	if err != nil {
		return fmt.Errorf("lead sla scan: %w", err)
	}
	if len(breaches) > 0 {
		log.InfoContext(wsCtx, "lead sla scan: breaches escalated", "count", len(breaches))
	}
	return nil
}
