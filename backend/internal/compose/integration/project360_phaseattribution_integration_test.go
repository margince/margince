// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The project's phase history names who made each move the way the record
// History does. It reads the audit row the writer wrote in the same
// transaction. Every project here is made and moved through the real store.

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// agentActing is the principal a passport presents for the human behind it,
// with the admin's grants so the project store accepts its writes.
func agentActing(e *Env, human, passport ids.UUID) context.Context {
	agent := identity.AgentIdentity{
		PassportID:  ids.From[ids.PassportKind](passport),
		WorkspaceID: ids.From[ids.WorkspaceKind](e.WS),
		OnBehalfOf:  ids.From[ids.UserKind](human),
		SeatType:    string(principal.SeatFull),
		Scopes:      principal.ScopeSet{principal.ScopeRead: {}, principal.ScopeWrite: {}},
		Permissions: AdminPerms,
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, agent.Principal())
}

func phaseHistoryOf(t *testing.T, e *Env, id ids.ProjectID) []crmcontracts.Project360PhaseTransition {
	t.Helper()
	page, err := project360Service(e, clocktest.Now(t).UTC()).Assemble(e.Admin(), id)
	if err != nil {
		t.Fatalf("assemble the project page: %v", err)
	}
	if page.PhaseHistory == nil {
		t.Fatal("phase_history absent")
	}
	return page.PhaseHistory.Data
}

func TestPhaseHistoryNamesTheAgentTheColleagueAndTheClient(t *testing.T) {
	e := Setup(t)
	colleague := seedWorkspaceUser(t, e, "Ada Authority")
	passport := seedGrantedPassport(t, e, colleague, "Claude")
	ctx := agentActing(e, colleague, passport)
	company := e.SeedCompany(t, "Agent GmbH", nil)

	made, err := e.Projects.CreateProject(ctx, projects.CreateProjectInput{
		Name: "Agent-run rollout", CompanyID: companyIDOf(company), Source: "manual",
	})
	if err != nil {
		t.Fatalf("create as an agent: %v", err)
	}
	id := projectIDOf(ids.UUID(made.Id))
	if _, err := e.Projects.AdvanceProjectPhase(ctx, id, projects.AdvanceProjectPhaseInput{ToPhase: projects.PhasePursuing}); err != nil {
		t.Fatalf("advance as an agent: %v", err)
	}

	rows := phaseHistoryOf(t, e, id)
	if len(rows) != 2 {
		t.Fatalf("history has %d rows, want the birth row and the move", len(rows))
	}
	for i, row := range rows {
		who := row.ChangedBy
		if who == nil || who.ActorType != crmcontracts.PrincipalAttributionActorTypeAgent ||
			who.OnBehalfOfName == nil || *who.OnBehalfOfName != "Ada Authority" ||
			who.AgentClient == nil || *who.AgentClient != "Claude" ||
			who.PassportId == nil || ids.UUID(*who.PassportId) != passport {
			t.Errorf("row %d changed_by = %+v, want an agent for Ada Authority via Claude", i, who)
		}
	}
	if rows[1].Reason != nil {
		t.Errorf("reason = %q, want none: the move was made without one", *rows[1].Reason)
	}
}

func TestPhaseHistoryNamesAHumanAndNeverInventsASystemMove(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Human GmbH", nil)
	p := seedProject(e.Admin(), t, e, "Human-run rollout", company, &e.Rep1)
	if _, err := e.Projects.AdvanceProjectPhase(e.Admin(), p.ID, projects.AdvanceProjectPhaseInput{ToPhase: projects.PhasePursuing}); err != nil {
		t.Fatal(err)
	}
	rows := phaseHistoryOf(t, e, p.ID)
	if who := rows[1].ChangedBy; who == nil || who.ActorType != crmcontracts.PrincipalAttributionActorTypeHuman ||
		who.ActorName == nil || who.OnBehalfOf != nil {
		t.Errorf("a human move reads %+v, want a named human", who)
	}

	// A move with a missing link, or a link to a gone audit row, reads as
	// unknown. The actor is absent; it is not a system actor.
	conn := OwnerConn(t)
	if _, err := conn.Exec(context.Background(),
		`UPDATE project_phase_history SET audit_log_id = NULL WHERE project_id = $1 AND from_phase IS NULL`, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(),
		`UPDATE project_phase_history SET audit_log_id = gen_random_uuid() WHERE project_id = $1 AND from_phase IS NOT NULL`, p.ID); err != nil {
		t.Fatal(err)
	}
	for i, row := range phaseHistoryOf(t, e, p.ID) {
		if row.ChangedBy != nil {
			t.Errorf("row %d changed_by = %+v, want absent (unknown) for an unresolvable link", i, row.ChangedBy)
		}
	}
}

func TestPhaseHistoryIgnoresALinkToAnotherProjectsAuditRow(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Crossed GmbH", nil)
	a := seedProject(e.Admin(), t, e, "Project A", company, &e.Rep1)
	b := seedProject(e.Admin(), t, e, "Project B", company, &e.Rep1)
	if _, err := e.Projects.AdvanceProjectPhase(e.Admin(), b.ID, projects.AdvanceProjectPhaseInput{ToPhase: projects.PhasePursuing}); err != nil {
		t.Fatal(err)
	}
	conn := OwnerConn(t)
	if _, err := conn.Exec(context.Background(),
		`UPDATE project_phase_history SET audit_log_id = (
		   SELECT h.audit_log_id FROM project_phase_history h WHERE h.project_id = $2 AND h.from_phase IS NOT NULL)
		 WHERE project_id = $1 AND from_phase IS NULL`, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	for i, row := range phaseHistoryOf(t, e, a.ID) {
		if row.ChangedBy != nil {
			t.Errorf("row %d changed_by = %+v, want absent for a link to another project's audit row", i, row.ChangedBy)
		}
	}
}

func TestPhaseHistoryNamesTheSystemOnlyForTheSystemPrincipal(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "System GmbH", nil)
	p := seedProject(e.Admin(), t, e, "System-run rollout", company, &e.Rep1)
	system := principal.SystemActing(e.Admin(), "system:phase-test")
	if _, err := e.Projects.AdvanceProjectPhase(system, p.ID, projects.AdvanceProjectPhaseInput{ToPhase: projects.PhasePursuing}); err != nil {
		t.Fatalf("advance as the system: %v", err)
	}
	rows := phaseHistoryOf(t, e, p.ID)
	if who := rows[1].ChangedBy; who == nil || who.ActorType != crmcontracts.PrincipalAttributionActorTypeSystem {
		t.Errorf("a system move reads %+v, want actor_type system", who)
	}
}
