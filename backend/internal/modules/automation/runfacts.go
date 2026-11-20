// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// What a run did, read from its trace: as English lines, and as fields a
// client words in the reader's language.

import (
	"context"
	"encoding/json"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// RecordNames answers the names of one type's records, leaving out each record
// the caller may not see.
type RecordNames interface {
	Labels(ctx context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error)
}

// WithRecordNames wires the names a run's target is labelled with. Without
// them no run carries a label.
func (h Handlers) WithRecordNames(names RecordNames) Handlers {
	h.names = names
	return h
}

// runActionResult is the run's English one-liner: the applied action kinds
// of a fired run, or the inbox for a staged one. Other outcomes speak through Reason.
func runActionResult(rec AutomationRunRecord) string {
	switch rec.Status {
	case runStatusApplied:
		kinds := runActionKinds(rec.Applied)
		if len(kinds) == 0 {
			return "applied"
		}
		return "applied " + strings.Join(kinds, ", ")
	case runStatusRequiresApproval:
		return "queued to approval inbox"
	default:
		return ""
	}
}

// runTarget is the record the run acted on: the first planned action's target.
// A run that failed before planning has none.
func runTarget(rec AutomationRunRecord) (datasource.EntityRef, bool) {
	actions := traceActions(rec.Planned)
	if len(actions) == 0 || actions[0].Target.ID == ids.Nil {
		return datasource.EntityRef{}, false
	}
	return actions[0].Target, true
}

// runTargetRef names the record the run acted on, as "type:id".
func runTargetRef(rec AutomationRunRecord) string {
	target, ok := runTarget(rec)
	if !ok {
		return ""
	}
	return string(target.Type) + ":" + target.ID.String()
}

func traceActions(trace json.RawMessage) []workflow.Action {
	var actions []workflow.Action
	if err := json.Unmarshal(trace, &actions); err != nil {
		return nil
	}
	return actions
}

// runActionKinds lists the distinct action kinds of a run trace, in trace
// order. A deduplicated create is its own entry: a sibling rule made that write.
func runActionKinds(trace json.RawMessage) []string {
	var kinds []string
	seen := map[string]bool{}
	for _, a := range traceActions(trace) {
		kind := string(a.Kind)
		if a.Deduplicated {
			kind += " (deduplicated)"
		}
		if !seen[kind] {
			seen[kind] = true
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// performedActions are the actions a run wrote, or for a queued run the ones
// it queued. A create a sibling rule already made is not this run's write.
func performedActions(rec AutomationRunRecord) []workflow.Action {
	var trace json.RawMessage
	switch rec.Status {
	case runStatusApplied:
		trace = rec.Applied
	case runStatusRequiresApproval:
		trace = rec.Planned
	}
	var performed []workflow.Action
	for _, a := range traceActions(trace) {
		if !a.Deduplicated {
			performed = append(performed, a)
		}
	}
	return performed
}

// describeRun sets the run's facts that need no reader's sight. Names wait for
// nameRunRecords.
func describeRun(rec AutomationRunRecord, run *crmcontracts.AutomationRun) {
	event := openapi_types.UUID(rec.TriggerEvent)
	run.EventId = &event
	if rec.TriggerType != "" {
		run.TriggerEventType = &rec.TriggerType
	}
	kinds := []string{}
	seen := map[workflow.ActionKind]bool{}
	for _, a := range performedActions(rec) {
		if !seen[a.Kind] {
			seen[a.Kind] = true
			kinds = append(kinds, string(a.Kind))
		}
	}
	run.ActionKinds = &kinds
	if target, ok := runTarget(rec); ok {
		run.Target = &crmcontracts.MagicEntityRef{Type: string(target.Type), Id: openapi_types.UUID(target.ID)}
	}
	if _, ok := createdTaskSubject(rec); ok {
		run.Created = &crmcontracts.AutomationRunCreatedRecord{Type: string(datasource.EntityActivity)}
	}
}

// createdTaskSubject is the subject of the first task a fired run created.
func createdTaskSubject(rec AutomationRunRecord) (string, bool) {
	if rec.Status != runStatusApplied {
		return "", false
	}
	for _, a := range performedActions(rec) {
		if a.Kind != workflow.ActionCreateTask {
			continue
		}
		var task struct {
			Subject string `json:"subject"`
		}
		if json.Unmarshal(a.Args, &task) == nil && task.Subject != "" {
			return task.Subject, true
		}
	}
	return "", false
}

// nameRunRecords labels each run's target where the reader may see it. The
// created task and the trigger's stage describe that record, so they go with its label.
func (h Handlers) nameRunRecords(ctx context.Context, recs []AutomationRunRecord, runs []crmcontracts.AutomationRun) error {
	if h.names == nil {
		return nil
	}
	wanted := map[string][]ids.UUID{}
	for _, run := range runs {
		if run.Target != nil {
			wanted[run.Target.Type] = append(wanted[run.Target.Type], ids.UUID(run.Target.Id))
		}
	}
	labels := map[string]map[ids.UUID]string{}
	for entityType, want := range wanted {
		named, err := h.names.Labels(ctx, entityType, want)
		if err != nil {
			return err
		}
		labels[entityType] = named
	}
	for i := range runs {
		target := runs[i].Target
		if target == nil {
			continue
		}
		label, seen := labels[target.Type][ids.UUID(target.Id)]
		if !seen || label == "" {
			continue
		}
		target.Label = &label
		runs[i].TriggerStageName = recs[i].ToStageName
		if subject, ok := createdTaskSubject(recs[i]); ok && runs[i].Created != nil {
			runs[i].Created.Label = &subject
		}
	}
	return nil
}
