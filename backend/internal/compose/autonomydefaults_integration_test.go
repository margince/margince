// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestLifecycleProposalsFollowTheOwnersDefaultAndOffChoice(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		name := "default on"
		if disabled {
			name = "saved off"
		}
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			company := seedAccountAtStage(t, e, "customer")
			prepareLifecycleOwner(t, e, company)
			signal := seedOpenContractEnded(t, e, company)
			if disabled {
				if _, err := approvals.NewService(e.DB()).SetAutoApply(e.As(e.Rep1, nil, integration.AdminPerms), lifecycleProposalKind, false); err != nil {
					t.Fatalf("switching lifecycle changes off: %v", err)
				}
			}
			if proposed := proposePass(t, e); proposed != 1 {
				t.Fatalf("proposed %d changes, want one", proposed)
			}
			applied, err := SweepAutoApply(e.Admin(), e.Pool)
			if err != nil {
				t.Fatalf("applying proposals: %v", err)
			}
			if disabled {
				if applied != 0 || accountStage(t, e, company) != "customer" || signalStatus(t, e, signal) != "open" {
					t.Fatal("a saved off choice allowed the lifecycle proposal to apply")
				}
				return
			}
			if applied != 1 || accountStage(t, e, company) != "former_customer" || signalStatus(t, e, signal) != "acknowledged" {
				t.Fatalf("default applied %d proposals, lifecycle %s, signal %s", applied, accountStage(t, e, company), signalStatus(t, e, signal))
			}
		})
	}
}

func prepareLifecycleOwner(t *testing.T, e *integration.Env, company ids.UUID) {
	t.Helper()
	e.WsExec(t, `UPDATE company SET owner_id = @owner WHERE id = @company`,
		pgx.NamedArgs{"owner": e.Rep1, "company": company})
	roleKey := "lifecycle-owner"
	e.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES (@key, 'Lifecycle owner', @permissions::jsonb)`,
		pgx.NamedArgs{"key": roleKey, "permissions": `{"objects":{"company":{"read":true,"update":true},"signal":{"read":true,"update":true},"activity":{"read":true},"contract":{"read":true},"project":{"read":true},"installation_settings":{"read":true}},"row_scope":"all"}`})
	e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id)
				SELECT id, @owner FROM role WHERE key = @key`, pgx.NamedArgs{"owner": e.Rep1, "key": roleKey})
}

func TestAutomaticTerminationRechecksANewlyActiveAgreement(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "customer")
	prepareLifecycleOwner(t, e, company)
	signal := seedOpenContractEnded(t, e, company)
	activate := prepareContinuityChange(t, e, company, "contract")
	if proposePass(t, e) != 1 {
		t.Fatal("fixture did not stage the initial termination")
	}
	if err := activate(e.Admin()); err != nil {
		t.Fatal(err)
	}
	applied, err := SweepAutoApply(e.Admin(), e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if applied != 0 || accountStage(t, e, company) != "customer" || signalStatus(t, e, signal) != "open" {
		t.Fatal("automatic termination ignored a current agreement")
	}
}
