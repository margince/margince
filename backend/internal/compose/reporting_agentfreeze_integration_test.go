// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A run an agent froze for its user publishes as the user's own does, because the
// sweep answers to the user the run names.
func TestTheSweepPublishesTheEditionAnAgentFroze(t *testing.T) {
	f := reportingBusiness(t)
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Agent's edition", Audience: "workspace", Selection: f.selection()})
	if err != nil {
		t.Fatal(err)
	}
	agent, ok := principal.Actor(f.human)
	if !ok {
		t.Fatal("no actor")
	}
	agent.Type, agent.ID = principal.PrincipalAgent, "agent:"+ids.NewV7().String()
	agent.PassportID, agent.OnBehalfOf = ids.NewV7(), agent.UserID
	run, err := f.service.Freeze(principal.WithActor(f.human, agent), ids.UUID(report.Id), report.Revision, "agent-freeze")
	if err != nil {
		t.Fatalf("the agent's freeze: %v", err)
	}
	if err := f.service.Sweep(principal.SystemActing(f.human, "system:agent-freeze")); err != nil {
		t.Fatal(err)
	}
	done, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != "succeeded" || done.EditionId == nil {
		t.Errorf("the run the agent froze ended as %+v, want it succeeded with an edition", done)
	}
}
