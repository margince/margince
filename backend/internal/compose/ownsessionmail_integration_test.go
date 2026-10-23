// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An approval the decider's own live conversation staged is not emailed to
// them, as the conversation shows it. The digest still lists it. Handed to a
// colleague since, it is emailed to them like any other.
func TestAnApprovalStagedInTheDecidersOwnSessionIsNotEmailed(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser)
	a.grantRole(t, a.e.Rep1)
	id, _ := a.stageCorrection(t)
	a.e.WsExec(t, `UPDATE approval SET requested_decider_id = $2 WHERE id = $1`, id, a.e.AdminUser)

	send := func(seat ids.UUID, use approvalNoticeUse) bool {
		t.Helper()
		attempt := notices.EmailAttempt{
			ApprovalID: &id, Notice: notices.Notice{Kind: notices.KindApprovalPending}, Recipient: ids.From[ids.UserKind](seat),
		}
		_, _, sent, err := refreshApprovalNotice(a.e.Admin(), a.e.Pool, attempt, clocktest.Now(t), use)
		if err != nil {
			t.Fatal(err)
		}
		return sent
	}
	if !send(a.e.AdminUser, forSingleEmail) {
		t.Fatal("an approval no conversation staged was not emailed")
	}
	a.e.WsExec(t, `UPDATE approval SET staged_in_conversation_of = $2 WHERE id = $1`, id, a.e.AdminUser)
	if send(a.e.AdminUser, forSingleEmail) {
		t.Error("an approval the decider's own conversation staged was emailed to them")
	}
	if !send(a.e.AdminUser, forDigestLine) {
		t.Error("the digest dropped an approval the decider's own conversation staged")
	}
	a.e.WsExec(t, `UPDATE approval SET requested_decider_id = $2 WHERE id = $1`, id, a.e.Rep1)
	if !send(a.e.Rep1, forSingleEmail) {
		t.Error("an approval handed to a colleague was not emailed to them")
	}
}
