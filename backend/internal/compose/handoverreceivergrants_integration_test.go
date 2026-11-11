// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestHandoverResolvesReceiverGrantsAfterTheMembershipLock(t *testing.T) {
	e := integration.Setup(t)
	memberManagerFor(t, e)
	e.WsExec(t, `INSERT INTO role_assignment(user_id,role_id) SELECT @user::uuid,id FROM role WHERE key='admin'`, pgx.StrictNamedArgs{"user": e.Rep2})
	lead := ids.UUID(seedBulkLeads(t, e, e.AdminUser, 1)[0].Id)
	handoff, err := e.Contacts.SubmitHandoff(e.Admin(), contacts.NewSDRHandoff{LeadID: &lead, AssignedTo: &e.Rep1})
	if err != nil {
		t.Fatal(err)
	}
	deactivate(t, e, e.Rep1)
	holder := integration.FreshOwnerConn(t)
	tx, err := holder.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(t.Context()); err != nil && err != pgx.ErrTxClosed {
			t.Error(err)
		}
	}()
	if _, err := tx.Exec(t.Context(), `SELECT id FROM app_user WHERE id=$1 FOR NO KEY UPDATE`, e.Rep2); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `DELETE FROM role_assignment WHERE user_id=$1`, e.Rep2); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO role_assignment(user_id,role_id) SELECT $1,id FROM role WHERE key='read_only'`, e.Rep2); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	type outcome struct {
		result crmcontracts.UserHandoverResult
		err    error
	}
	completed := make(chan outcome, 1)
	go func() {
		result, err := handoverSeamFor(e).HandOver(integration.OnSeat(e.Admin(), principal.SeatFull), e.Rep1, e.Rep2)
		completed <- outcome{result, err}
		close(done)
	}()
	testdb.AwaitQueuedBehind(t, tx, int32(holder.PgConn().PID()), 1, done, "handover")
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	result := <-completed
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.result.Moved.SdrHandoff != 0 || result.result.Left.SdrHandoff != 1 {
		t.Fatalf("used grants from before demotion: %+v", result.result)
	}
	if got := e.WsScalar(t, `SELECT assigned_to::text FROM sdr_handoff WHERE id=$1`, handoff); got != e.Rep1.String() {
		t.Fatal("demoted receiver acquired the prospect")
	}
}
