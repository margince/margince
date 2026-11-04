// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A user's own session and an agent on their passport are that user in the audit
// trail. Automatic work is not, even when on_behalf_of names the user whose
// decision it applies.

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAWorkedTaskAndAMachineRecordReadTheActorThroughThePassport(t *testing.T) {
	e := Setup(t)
	passport := ids.NewV7()
	for _, tc := range []struct {
		name               string
		actorType, actorID string
		passport           *ids.UUID
		wantWorked         bool
	}{
		{"a user", "human", "human:" + e.Rep1.String(), nil, true},
		{"a user's agent", "agent", "agent:" + passport.String(), &passport, true},
		{"automatic work for a user", "agent", "agent:site-lead-accept", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := ids.NewV7()
			e.WsExec(t, `
				INSERT INTO audit_log (id, actor_type, actor_id, passport_id, on_behalf_of, action, entity_type, entity_id, after, occurred_at)
				VALUES ($1, $2, $3, $4, $5, 'update', 'activity', $6, '{"is_done":true}', now())`,
				ids.NewV7(), tc.actorType, tc.actorID, tc.passport, e.Rep1, task)
			var worked bool
			if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
				return tx.QueryRow(e.Admin(), `SELECT `+storekit.HumanWroteActivityClause("a")+` FROM (SELECT $1::uuid AS id) a`, task).Scan(&worked)
			}); err != nil {
				t.Fatal(err)
			}
			if worked != tc.wantWorked {
				t.Errorf("HumanWroteActivityClause = %t, want %t", worked, tc.wantWorked)
			}
		})
	}
	for _, tc := range []struct {
		name          string
		creatorID     string
		passport      *ids.UUID
		wantUntouched bool
	}{
		{"created by a user's agent", "agent:" + passport.String(), &passport, false},
		{"created by automatic work", "agent:deal-scout", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := ids.NewV7()
			e.WsExec(t, `
				INSERT INTO audit_log (id, actor_type, actor_id, passport_id, action, entity_type, entity_id, occurred_at)
				VALUES ($1, 'agent', $2, $3, 'create', 'activity', $4, now())`,
				ids.NewV7(), tc.creatorID, tc.passport, record)
			var untouched bool
			if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) (err error) {
				untouched, err = storekit.MachineCreatedUntouched(e.Admin(), tx, "activity", record)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if untouched != tc.wantUntouched {
				t.Errorf("MachineCreatedUntouched = %t, want %t", untouched, tc.wantUntouched)
			}
		})
	}
}
