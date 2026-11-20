// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
)

// stageRuleFixture is a deal moved into the stage a create-task rule watches,
// with the rule's one run recorded by the real engine.
type stageRuleFixture struct {
	rule, deal, stageName, eventID string
}

func moveADealIntoAWatchedStage(t *testing.T, e *apptest.AppEnv) stageRuleFixture {
	t.Helper()
	var pipelines struct {
		Data []struct {
			ID        string `json:"id"`
			IsDefault bool   `json:"is_default"`
			Stages    []struct {
				ID, Name, Semantic string
			} `json:"stages"`
		} `json:"data"`
	}
	mustCall(t, e, "GET", "/v1/pipelines", nil, http.StatusOK, &pipelines)
	var pipeline string
	var open []struct{ ID, Name, Semantic string }
	for _, p := range pipelines.Data {
		if p.IsDefault {
			pipeline = p.ID
			for _, s := range p.Stages {
				if s.Semantic == "open" {
					open = append(open, s)
				}
			}
		}
	}
	if len(open) < 2 {
		t.Fatalf("the default pipeline has %d open stages, want two to move between", len(open))
	}
	watched := open[1]
	var rule crmcontracts.Automation
	mustCall(t, e, "POST", "/v1/automations", AnyMap{"key": "stage_change_create_task", "name": "Next step", "params": AnyMap{
		"pipeline_id": pipeline, "stage_ids": []string{watched.ID}, "task_title": "Send the proposal", "due_in_days": 2,
	}}, http.StatusCreated, &rule)
	if status := e.Call(t, "PATCH", "/v1/automations/"+rule.Id.String(), AnyMap{"status": "enabled"},
		map[string]string{"If-Match": strconv.Itoa(*rule.Version)}, nil); status != http.StatusOK {
		t.Fatalf("enable the rule → %d", status)
	}
	deal := createdRecord(t, e, "/v1/deals", AnyMap{"name": "Globex Renewal", "pipeline_id": pipeline, "stage_id": open[0].ID, "source": "manual"})
	mustCall(t, e, "POST", "/v1/deals/"+deal+"/advance", AnyMap{"to_stage_id": watched.ID}, http.StatusOK, nil)

	var raw []byte
	if err := e.Pool.QueryRow(context.Background(), `SELECT envelope::text FROM event_outbox
		WHERE envelope->>'type' = 'deal.stage_changed' AND envelope->'entity'->>'id' = $1 ORDER BY seq DESC LIMIT 1`, deal).Scan(&raw); err != nil {
		t.Fatalf("reading the stage move's event: %v", err)
	}
	var env kevents.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if err := compose.NewWorkflowEngine(e.DB()).HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("the engine refused the stage move: %v", err)
	}
	return stageRuleFixture{rule: rule.Id.String(), deal: deal, stageName: watched.Name, eventID: env.EventID.String()}
}

func onlyRun(t *testing.T, e *apptest.AppEnv, rule string) crmcontracts.AutomationRun {
	t.Helper()
	var page struct {
		Data []crmcontracts.AutomationRun `json:"data"`
	}
	mustCall(t, e, "GET", "/v1/automations/"+rule+"/runs", nil, http.StatusOK, &page)
	if len(page.Data) != 1 {
		t.Fatalf("the rule has %d runs, want the one the stage move fired", len(page.Data))
	}
	return page.Data[0]
}

func TestARunNamesItsRecordTheTaskItCreatedAndTheStageThatFiredIt(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	f := moveADealIntoAWatchedStage(t, e)

	run := onlyRun(t, e, f.rule)
	if run.Outcome != "fired" || run.EventId == nil || run.EventId.String() != f.eventID {
		t.Fatalf("run = %s event %v, want fired by %s", run.Outcome, run.EventId, f.eventID)
	}
	requireStr(t, "trigger_event_type", run.TriggerEventType, "deal.stage_changed")
	requireStr(t, "trigger_stage_name", run.TriggerStageName, f.stageName)
	if run.ActionKinds == nil || !slices.Equal(*run.ActionKinds, []string{"create_task"}) {
		t.Fatalf("action_kinds = %v, want [create_task]", run.ActionKinds)
	}
	if run.Target == nil || run.Target.Type != "deal" || run.Target.Id.String() != f.deal {
		t.Fatalf("target = %+v, want the deal %s", run.Target, f.deal)
	}
	requireStr(t, "target label", run.Target.Label, "Globex Renewal")
	if run.Created == nil || run.Created.Type != "activity" {
		t.Fatalf("created = %+v, want the task", run.Created)
	}
	requireStr(t, "created label", run.Created.Label, "Send the proposal")
	requireStr(t, "target_ref", run.TargetRef, "deal:"+f.deal)
	requireStr(t, "action_result", run.ActionResult, "applied create_task")
}

func TestARunWhoseRecordTheReaderCannotSeeCarriesNoNames(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	f := moveADealIntoAWatchedStage(t, e)
	var role struct {
		Key string `json:"key"`
	}
	mustCall(t, e, "POST", "/v1/roles", AnyMap{"copy_from": "rep", "name": "No deals"}, http.StatusCreated, &role)
	mustCall(t, e, "PATCH", "/v1/roles/"+role.Key+"/objects/deal",
		AnyMap{"create": false, "read": false, "update": false, "delete": false}, http.StatusOK, nil)
	const email, password = "rep@run-facts.test", "run-facts-rep-password"
	rep := createdRecord(t, e, "/v1/users", AnyMap{"email": email, "display_name": "Rae Rep", "role": role.Key})
	apptest.SetPasswordByLink(t, e, rep, password)
	reader := e.AsSecondSeat(t, email, password)
	if status := reader.Call(t, "GET", "/v1/deals/"+f.deal, nil, nil, nil); status == http.StatusOK {
		t.Fatal("the reader opened the deal, want it hidden from them")
	}

	run := onlyRun(t, reader, f.rule)
	if run.Target == nil || run.Target.Id.String() != f.deal || run.Target.Label != nil {
		t.Fatalf("target = %+v, want the deal's id with no name", run.Target)
	}
	if run.Created == nil || run.Created.Label != nil || run.TriggerStageName != nil {
		t.Fatalf("created = %+v stage = %v, want neither named", run.Created, run.TriggerStageName)
	}
	requireStr(t, "trigger_event_type", run.TriggerEventType, "deal.stage_changed")
}
