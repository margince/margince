// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func TestAStagedActionKeepsTheRuleAndTriggerInEveryLanguage(t *testing.T) {
	at := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for _, trigger := range []workflow.Trigger{{EventType: "deal.stage_changed"}, {Schedule: "clock:review"}} {
		ev := workflow.Event{AutomationName: "Quarterly review", Type: trigger.EventType, OccurredAt: at}
		why := firingReason(workflow.Spec{Name: "review", Trigger: trigger}, ev)
		fake := &fakeApprovals{id: ids.New[ids.ApprovalKind]()}
		_, err := ApplyActions(context.Background(), Executors{Approvals: fake}, workflow.Effect{
			Why: why,
			Actions: []workflow.Action{{
				Kind:   workflow.ActionEmitFlowEvent,
				Target: datasource.EntityRef{Type: datasource.EntityDeal, ID: ids.NewV7()},
				Args:   json.RawMessage(`{"question":"Has the account review been completed?"}`),
			}},
		})
		var staged *workflow.StagedApprovalError
		if !errors.As(err, &staged) || len(fake.calls) != 1 || fake.calls[0].Why == nil {
			t.Fatalf("staged action lost its reason: calls=%+v, err=%v", fake.calls, err)
		}
		for _, language := range textlang.Shipped {
			text, err := fake.calls[0].Why.Render(language)
			if err != nil {
				t.Fatal(err)
			}
			kind := trigger.EventType + trigger.Schedule
			for _, want := range []string{ev.AutomationName, kind, at.Format(time.RFC3339)} {
				if !strings.Contains(text, want) {
					t.Errorf("%s reason %q omits %q", language, text, want)
				}
			}
		}
	}
}

func TestALongRuleNameKeepsItsReasonAndTrigger(t *testing.T) {
	at := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	why := firingReason(workflow.Spec{Name: "review"}, workflow.Event{
		AutomationName: strings.Repeat("界", 1000), Type: "deal.stage_changed", OccurredAt: at,
	})
	for _, language := range textlang.Shipped {
		text, err := why.Render(language)
		if err != nil || !strings.Contains(text, "deal.stage_changed") || !strings.Contains(text, at.Format(time.RFC3339)) {
			t.Fatalf("long-name reason in %s = %q, err=%v", language, text, err)
		}
	}
}

func TestBothClockEventBuildersKeepTheStoredRuleName(t *testing.T) {
	at := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	inst := automationInstance{id: ids.New[ids.AutomationKind](), name: "Quarterly review"}
	activity, err := buildActivityAnchorEvent(ids.NewV7(), at, inst, EntityAnchor{Anchor: at})
	if err != nil {
		t.Fatal(err)
	}
	date, err := buildDateFieldAnchorEvent(ids.NewV7(), at, inst, DateFieldAnchor{Anchor: at})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []workflow.Event{activity, date} {
		if event.AutomationName != inst.name {
			t.Fatalf("clock event lost the rule name: %+v", event)
		}
	}
}

func TestAHeldDraftKeepsItsFiringReason(t *testing.T) {
	fake := &fakeApprovals{id: ids.New[ids.ApprovalKind]()}
	why := firingReason(workflow.Spec{Name: "recap"}, workflow.Event{Type: "activity.captured", OccurredAt: time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)})
	_, err := stageHeldDraft(context.Background(), fake, nil, datasource.EntityRef{Type: datasource.EntityContact, ID: ids.NewV7()}, HeldDraftProposal{To: "buyer@example.com"}, why)
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.calls) != 1 || fake.calls[0].Why != why {
		t.Fatalf("held draft lost the reason: %+v", fake.calls)
	}
}
