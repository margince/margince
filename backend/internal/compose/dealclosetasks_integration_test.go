// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Closing a deal through the tool surface asks about its open tasks.
//
// A close never settles tasks silently, and the close itself asks. That
// question lived only on the web, so the surface agents use could not put it.

import (
	"encoding/json"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// closeTasksProbe is one deal carrying one open task, and the surface that
// closes it.
type closeTasksProbe struct {
	e        *integration.Env
	registry *agents.Registry
	deal     ids.UUID
	task     ids.UUID
	won      ids.StageID
}

func newCloseTasksProbe(t *testing.T, name string) closeTasksProbe {
	t.Helper()
	e := integration.Setup(t)
	pipeline, open, won := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, name, pipeline, open, &e.Rep1)

	task := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, is_done, source, captured_by)
		VALUES ($1, 'task', 'Send the signed copy', now(), false, 'manual', 'human:x')`, task)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`,
		task, deal)

	native := NewProvider(e.Pool)
	registry := agents.NewRegistry(
		approvalsAdapter{svc: approvals.NewService(e.DB())}, auth.NewGate(adminSeat{}),
		agents.WithDealTaskSettler(newDealCloseTasks(activities.NewStore(e.DB()), e.DB())))
	agents.RegisterCoreTools(registry, native, native, nil, nil, nil, nil)
	return closeTasksProbe{e: e, registry: registry, deal: ids.UUID(deal), task: task, won: won}
}

// close runs the close through the tool and answers its result, which is where
// the question about the deal's open tasks is answered.
//
// It executes rather than staging. Confirm-first binds an agent, and this
// caller is a human holding the grant, the authority the web dialog runs under.
func (p closeTasksProbe) close(t *testing.T, settle string) crmcontracts.ProgressDealResult {
	t.Helper()
	ctx := p.e.As(p.e.Rep1, []ids.UUID{p.e.Team1}, integration.AdminPerms)
	args := map[string]any{
		"deal_id":                     p.deal.String(),
		"to_stage_id":                 p.won.String(),
		"won_without_contract_reason": integration.WonByImport(),
	}
	if settle != "" {
		args["settle"] = settle
	}
	call, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("encoding the call: %v", err)
	}
	raw, err := p.registry.Invoke(ctx, "progress_deal", call)
	if err != nil {
		t.Fatalf("the close answered %v", err)
	}
	var env agents.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("the close's answer is not an envelope: %v", err)
	}
	var result crmcontracts.ProgressDealResult
	if err := json.Unmarshal(env.Data, &result); err != nil {
		t.Fatalf("decoding the close's answer: %v", err)
	}
	return result
}

func (p closeTasksProbe) taskIsDone(t *testing.T) bool {
	t.Helper()
	return p.e.WsScalar(t, `SELECT is_done::text FROM activity WHERE id = $1`, p.task) == "true"
}

// Keep is the answer a caller gives by saying nothing: the tasks are named, and
// left where they are.
func TestClosingADealThroughTheToolNamesTheTasksItLeavesOpen(t *testing.T) {
	p := newCloseTasksProbe(t, "Kept-tasks renewal")
	result := p.close(t, "")

	named := openTasksOf(result)
	if len(named) != 1 || ids.UUID(named[0].Id) != p.task {
		t.Fatalf("the close named %+v, want the one open task %s", named, p.task)
	}
	if named[0].Subject != "Send the signed copy" {
		t.Errorf("the named task reads %q, want the task's own subject", named[0].Subject)
	}
	if len(settledBy(result)) != 0 {
		t.Errorf("the close settled %v, and nothing asked it to", settledBy(result))
	}
	if p.taskIsDone(t) {
		t.Error("the task was completed by a close that was not asked to settle it")
	}
}

// Complete is the other answer. It goes through the write the bulk change
// applies, so the task carries the done state the web leaves.
func TestClosingADealThroughTheToolCompletesItsTasksWhenAsked(t *testing.T) {
	p := newCloseTasksProbe(t, "Settled-tasks renewal")
	result := p.close(t, "complete")

	settled := settledBy(result)
	if len(settled) != 1 || ids.UUID(settled[0]) != p.task {
		t.Fatalf("the close settled %+v, want the one open task %s", settled, p.task)
	}
	if named := openTasksOf(result); len(named) != 0 {
		t.Errorf("the close named %+v as still open after settling them", named)
	}
	if !p.taskIsDone(t) {
		t.Error("the task is still open after a close that was asked to complete it")
	}
}

// The contract makes both lists optional. Absent and empty are one claim here,
// so the tests read them the same way.
func openTasksOf(r crmcontracts.ProgressDealResult) []crmcontracts.DealOpenTask {
	if r.OpenTasks == nil {
		return nil
	}
	return *r.OpenTasks
}

func settledBy(r crmcontracts.ProgressDealResult) []openapi_types.UUID {
	if r.SettledTaskIds == nil {
		return nil
	}
	return *r.SettledTaskIds
}
