// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The automation_health lane: rules that stopped doing their work, withheld
// for readers the automation screens refuse.

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubAutomations struct {
	rows  []TroubledAutomationRun
	err   error
	since time.Time
}

func (s *stubAutomations) TroubledRuns(_ context.Context, since time.Time, _ int) ([]TroubledAutomationRun, error) {
	s.since = since
	return s.rows, s.err
}

func automationLaneService(health AutomationHealth) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, health, nil, fixedClock)
}

func TestATroubledFiringNamesItsRuleAndItsReason(t *testing.T) {
	fired := readInstant.Add(-2 * time.Hour)
	stub := &stubAutomations{rows: []TroubledAutomationRun{
		{ID: ids.NewV7(), Name: "Route new leads", Outcome: "failed", Reason: "the assignee seat is gone", OccurredAt: fired},
		{ID: ids.NewV7(), Name: "Renewal reminder", Outcome: "blocked", OccurredAt: fired},
	}}
	out, err := automationLaneService(stub).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.AutomationHealth == nil || len(*out.AutomationHealth) != 2 {
		t.Fatalf("automation_health carries %v, want two firings", out.AutomationHealth)
	}
	if want := readInstant.Add(-7 * 24 * time.Hour); !stub.since.Equal(want) {
		t.Errorf("window opens at %v, want %v", stub.since, want)
	}
	failed := (*out.AutomationHealth)[0]
	if failed.Title == nil || *failed.Title != "Route new leads" {
		t.Errorf("title = %v, want the rule's own name", failed.Title)
	}
	if failed.Kind == nil || *failed.Kind != "failed" {
		t.Errorf("kind = %v, want the closed failed/blocked vocabulary", failed.Kind)
	}
	if failed.Detail == nil || *failed.Detail != "the assignee seat is gone" {
		t.Errorf("detail = %v, want the engine's recorded reason", failed.Detail)
	}
	blocked := (*out.AutomationHealth)[1]
	if blocked.Detail != nil {
		t.Errorf("a firing with no recorded reason invented one: %q", *blocked.Detail)
	}
	if len(failed.Actions)+len(blocked.Actions) != 0 {
		t.Error("a rule-health card promises no verbs")
	}
}

func TestARefusedAutomationReadIsNamedAsWithheld(t *testing.T) {
	out, err := automationLaneService(&stubAutomations{err: apperrors.ErrPermissionDenied}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.AutomationHealth != nil {
		t.Fatalf("automation_health = %v, want the lane withheld", out.AutomationHealth)
	}
	if out.LanesOmitted == nil || !slices.Contains(*out.LanesOmitted, crmcontracts.AttentionLanesOmitted("automation_health")) {
		t.Fatal("a refused automation read was not named in lanes_omitted")
	}
}

// Every rule doing its work is an EMPTY lane — the feed looked — which the
// absent lane never promises.
func TestHealthyAutomationsReadAsAnEmptyLane(t *testing.T) {
	out, err := automationLaneService(&stubAutomations{}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.AutomationHealth == nil || len(*out.AutomationHealth) != 0 {
		t.Fatalf("automation_health = %v, want an empty lane", out.AutomationHealth)
	}
}
