// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
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
			e.WsExec(t, `UPDATE company SET owner_id = @owner WHERE id = @company`,
				pgx.NamedArgs{"owner": e.Rep1, "company": company})
			roleKey := "lifecycle-owner"
			e.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES (@key, 'Lifecycle owner', @permissions::jsonb)`,
				pgx.NamedArgs{"key": roleKey, "permissions": `{"objects":{"company":{"read":true,"update":true},"signal":{"read":true,"update":true},"installation_settings":{"read":true}},"row_scope":"all"}`})
			e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id)
				SELECT id, @owner FROM role WHERE key = @key`, pgx.NamedArgs{"owner": e.Rep1, "key": roleKey})
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
