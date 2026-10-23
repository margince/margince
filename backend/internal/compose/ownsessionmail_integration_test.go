// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An approval the decider's own connected AI client staged is not emailed to
// them: the conversation shows it. The digest still lists it, and one staged
// with no connection, as an overnight run's, is still emailed.
func TestAnApprovalStagedInTheDecidersOwnSessionIsNotEmailed(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser)
	passport := a.e.SeedPassport(t, integration.OwnerConn(t), "own session probe")
	var grant ids.UUID
	if err := database.WithWorkspaceTx(a.e.Admin(), a.e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `INSERT INTO oauth_client (client_id, client_name, redirect_uris, created_via)
			VALUES ('own-session', 'Claude', ARRAY['https://x.example/cb'], 'admin')`); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(), `INSERT INTO oauth_grant (client_id, user_id, scopes)
			VALUES ('own-session', $1, ARRAY['write']) RETURNING id`, a.e.AdminUser).Scan(&grant)
	}); err != nil {
		t.Fatal(err)
	}
	id, _ := a.stageCorrection(t)
	a.e.WsExec(t, `UPDATE approval SET on_behalf_of = $2, requested_decider_id = $2, passport_id = $3 WHERE id = $1`,
		id, a.e.AdminUser, passport)

	attempt := notices.EmailAttempt{
		ApprovalID: &id, Notice: notices.Notice{Kind: notices.KindApprovalPending}, Recipient: ids.From[ids.UserKind](a.e.AdminUser),
	}
	send := func(use approvalNoticeUse) bool {
		t.Helper()
		_, _, sent, err := refreshApprovalNotice(a.e.Admin(), a.e.Pool, attempt, clocktest.Now(t), use)
		if err != nil {
			t.Fatal(err)
		}
		return sent
	}
	if !send(forSingleEmail) {
		t.Fatal("an approval staged with no connection was not emailed")
	}
	a.e.WsExec(t, `UPDATE approval SET staged_by_connection = $2 WHERE id = $1`, id, grant)
	if send(forSingleEmail) {
		t.Error("an approval the decider's own AI client staged was emailed to them")
	}
	if !send(forDigestLine) {
		t.Error("the digest dropped an approval the decider's own AI client staged")
	}
}
