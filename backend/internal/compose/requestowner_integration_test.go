// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// proposePassAsTheJob runs the lifecycle pass under the principal the nightly
// job binds, which names no human of its own.
func proposePassAsTheJob(t *testing.T, e *integration.Env) int {
	t.Helper()
	proposer := NewSignalProposer(e.Pool, approvalsServiceWithEffects(e.Pool), slog.New(slog.DiscardHandler))
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), signalScanActor)
	staged, err := proposer.RunWorkspace(ctx)
	if err != nil {
		t.Fatalf("RunWorkspace: %v", err)
	}
	return staged
}

// A background finding on a record nobody owns asks nobody: no request, and so
// no notice to the colleagues who could have decided it.
func TestAContractEndedOnACompanyNobodyOwnsAsksNobody(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "customer")
	e.WsExec(t, `UPDATE company SET owner_id = NULL WHERE id = $1`, company)
	seedOpenContractEnded(t, e, company)

	if staged := proposePassAsTheJob(t, e); staged != 0 {
		t.Fatalf("the pass staged %d offers on a company nobody owns, want none", staged)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval`); n != 0 {
		t.Fatalf("%d approvals exist for a company nobody owns", n)
	}
}

// The same finding on an owned company is for its owner and nobody else.
func TestAContractEndedIsForTheCompanysOwner(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "customer")
	seedOpenContractEnded(t, e, company)

	if staged := proposePassAsTheJob(t, e); staged != 1 {
		t.Fatalf("the pass staged %d offers, want one", staged)
	}
	if got := e.WsScalar(t, `SELECT requested_decider_id::text FROM approval WHERE target_entity_id = $1`, company); got != e.AdminUser.String() {
		t.Fatalf("the offer is for %s, want the company's owner %s", got, e.AdminUser)
	}
}
