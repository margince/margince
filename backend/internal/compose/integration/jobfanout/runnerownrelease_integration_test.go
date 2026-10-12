// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// releasingSpecName is a scheduled agent allowed to answer approvals, which no
// shipped agent is: it is how this lane reaches decide_approval from a run.
const releasingSpecName = "e2e_releasing_agent"

var releasingSpec = runner.AgentSpec{
	Name:       releasingSpecName,
	Goal:       "Answer the approvals waiting on this workspace.",
	DueHourUTC: 2,
	Tools:      []string{"decide_approval"},
}

// A scheduled run has nobody behind it to have said yes, so it never releases
// a proposal its own credential staged — even the record change the same
// credential could release from an attended conversation. What makes the run
// unattended is the run id the runner stamps on every tool call it makes.
func TestAScheduledRunDoesNotReleaseTheChangeItsCredentialProposed(t *testing.T) {
	re := setupRunner(t)

	var company struct {
		ID string `json:"id"`
	}
	if status := re.Call(t, "POST", "/v1/companies", integration.AnyMap{"source": "manual", "display_name": "Runheld Co"}, nil, &company); status != http.StatusCreated {
		t.Fatalf("create company → %d", status)
	}
	approvalID := re.stageAsThePassport(t, company.ID)

	trigger := releasingSpecName + ":e2e-own-release"
	re.brain.Script(
		fmt.Sprintf(`{"tool":"decide_approval","args":{"staged_action_id":"%s","decision":"approve"}}`, approvalID),
		`{"final":{"summary":"tried to answer one approval"}}`,
	)
	re.enqueue(t, releasingSpecName, trigger, &re.passportID)
	re.tick(t)

	_, trace, _ := re.runRow(t, trigger)
	if len(trace) == 0 || trace[0].Tool != "decide_approval" || trace[0].Admission != runner.AdmissionRefused ||
		!strings.Contains(trace[0].Observation, "proposed the action") {
		t.Fatalf("the run never had its own release refused: trace = %+v", trace)
	}
	var status string
	if err := database.WithWorkspaceTx(re.wsCtx, re.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT status FROM approval WHERE id = $1`, approvalID).Scan(&status)
	}); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("a scheduled run released its own credential's proposal: approval is %q, want pending", status)
	}
}

// stageAsThePassport stages an undoable record change on the runner's own
// passport, the way its attended twin would have proposed it.
func (re *runnerEnv) stageAsThePassport(t *testing.T, companyID string) ids.ApprovalID {
	t.Helper()
	agent, err := identity.NewService(re.pool).AuthenticateAgentByID(re.wsCtx, re.passportID)
	if err != nil {
		t.Fatal(err)
	}
	target, err := ids.Parse(companyID)
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithActor(re.wsCtx, agent.Principal())
	id, err := approvals.NewService(database.BindTo(re.pool, ids.From[ids.WorkspaceKind](re.wsID))).Stage(ctx, approvals.StageInput{
		Kind:           "update_record",
		ProposedChange: []byte(`{"display_name":"Runheld Global"}`),
		DiffHash:       "runheld-" + companyID,
		TargetType:     "company",
		TargetID:       target,
		Summary:        "Rename Runheld Co?",
	})
	if err != nil {
		t.Fatalf("staging on the runner's passport: %v", err)
	}
	return id
}
