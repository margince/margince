// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A queued notice about a proposed commitment task is checked against the
// promise at send time. Once the promise is settled, the proposal is stale and
// nothing is mailed; while it is open, the notice goes out.
func TestAQueuedCommitmentNoticeIsNotMailedOnceThePromiseIsSettled(t *testing.T) {
	e, wire := stageMailedRequest(t)
	claim, err := ids.Parse(e.WsScalar(t, `SELECT coalesce(proposed_change->>'claim_id', proposed_change->'claim_ids'->>0) FROM approval WHERE id = $1`, ids.UUID(wire.Id)))
	if err != nil {
		t.Fatalf("the staged proposal names no promise: %v", err)
	}
	id := ids.From[ids.ApprovalKind](ids.UUID(wire.Id))
	attempt := notices.EmailAttempt{ApprovalID: &id, Notice: notices.Notice{Kind: notices.KindApprovalPending}, Recipient: ids.From[ids.UserKind](e.Rep2)}
	reader := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, _, send, err := refreshApprovalNotice(reader, e.Pool, attempt, clocktest.Now(t), forSingleEmail); err != nil || !send {
		t.Fatalf("the notice about an open promise was not sent: send=%v err=%v", send, err)
	}
	if err := e.Contacts.SettleConversationClaim(e.Admin(), claim, "dismissed"); err != nil {
		t.Fatal(err)
	}
	if _, _, send, err := refreshApprovalNotice(reader, e.Pool, attempt, clocktest.Now(t), forSingleEmail); err != nil || send {
		t.Fatalf("a notice about a dismissed promise was mailed: send=%v err=%v", send, err)
	}
}
