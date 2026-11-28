// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The FX refresh acts for the admin who asked, under that admin's live grants.
// It stages its proposals as their agent. An admin who may no longer read the
// rate sheet gets a failed job and no proposal. An approved rate is written by
// the same agent, acting for the admin who approved it.

import (
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func fxRefreshJobFor(e *integration.Env, requester ids.UUID) *river.Job[FxRateRefreshArgs] {
	return &river.Job[FxRateRefreshArgs]{
		JobRow: &rivertype.JobRow{},
		Args:   FxRateRefreshArgs{RequestedBy: principal.HumanIDPrefix + requester.String()},
	}
}

func TestAnFxRefreshStagesItsProposalAsTheAdminsAgent(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "admin")
	worker := &fxRefreshWorker{
		refresh: fxRefreshWith(e, fxReply(pair("EUR", "USD", "1.08")), []string{"USD"}),
		users:   identity.NewService(e.Pool),
	}

	if err := worker.Work(boundBackground(t), fxRefreshJobFor(e, e.Rep1)); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'fx_rate_proposal'
		AND proposed_by = 'agent:rate-refresh' AND on_behalf_of = $1 AND passport_id IS NULL`, e.Rep1); n != 1 {
		t.Fatalf("%d proposals staged by agent:rate-refresh for Rep1 with no passport, want 1", n)
	}
}

func TestAnFxRefreshForAnAdminWithoutTheGrantFailsAndStagesNothing(t *testing.T) {
	e := integration.Setup(t)
	worker := &fxRefreshWorker{
		refresh: fxRefreshWith(e, fxReply(pair("EUR", "USD", "1.08")), []string{"USD"}),
		users:   identity.NewService(e.Pool),
	}

	if err := worker.Work(boundBackground(t), fxRefreshJobFor(e, e.Rep1)); err == nil {
		t.Fatal("a refresh for a seat with no grant on the rate sheet succeeded, want the job to fail where it can be seen")
	}
	if n := fxPending(t, e); n != 0 {
		t.Fatalf("%d proposals staged for a seat with no grant on the rate sheet, want 0", n)
	}
}

func TestAnApprovedFxRateIsWrittenByTheAgentForTheDecider(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	svc := rateSvc(t, e)
	id := stageProposal(ctx, t, svc, fxRateProposalKind, fxRateTargetType, e.WS,
		map[string]string{"from_currency": "USD", "rate": "0.9"}, "USD → EUR 0.9")

	if _, err := svc.Decide(ctx, id, true, nil); err != nil {
		t.Fatalf("approve: %v", err)
	}
	rows, err := deals.NewStore(e.DB(), DealsInstallation()).ListLatestFxRates(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rate sheet after approval: %+v (%v), want the one approved row", rows, err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'fx_rate'
		AND actor_type = 'agent' AND actor_id = 'agent:rate-refresh' AND on_behalf_of = $1`, e.Rep1); n != 1 {
		t.Errorf("%d rate writes by agent:rate-refresh for the decider, want 1", n)
	}
}
