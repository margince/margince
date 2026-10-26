// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Notices queued before a request named its one decider, or for a colleague it
// no longer names, must not leave the product. The request is moved by SQL. No
// writer today queues a notice for anyone but the decider, so this is the state
// such a notice is left in.
func moveDecider(t *testing.T, approvalID ids.ApprovalID, to ids.UUID) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE approval SET requested_decider_id = $2 WHERE id = $1`, approvalID, to); err != nil {
		t.Fatalf("moving the request to another colleague: %v", err)
	}
}

func TestANoticeQueuedForSomebodyNoLongerAskedIsNotMailed(t *testing.T) {
	e := setupNoticeMail(t)
	e.grantRole(t, e.e.AdminUser)
	e.grantRole(t, e.e.Rep2)
	approvalID, env := e.stageCorrection(t)
	e.deliver(t, env)
	rows := e.delivered(t)
	if len(rows) != 1 || rows[0].recipient != e.e.AdminUser {
		t.Fatalf("want one notice, for the deal's owner; got %+v", rows)
	}

	moveDecider(t, approvalID, e.e.Rep2)
	e.work(t, rows[0].id)

	if got := e.relay.count(); got != 0 {
		t.Fatalf("a notice for a colleague the request no longer names was mailed %d time(s)", got)
	}
}

// The digest and the immediate mail both ask refreshApprovalNotice whether a
// notice is still current, and the digest leaves out any that is not. So the
// check itself is the one place a stale recipient is stopped for both.
func TestTheSendTimeCheckKeepsOnlyTheDecidersNotice(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser)
	a.grantRole(t, a.e.Rep2)
	approvalID, env := a.stageCorrection(t)
	a.deliver(t, env)

	current := func(seat ids.UUID) bool {
		t.Helper()
		attempt := notices.EmailAttempt{
			ApprovalID: &approvalID,
			Notice:     notices.Notice{Kind: notices.KindApprovalPending, Subject: stagedSummary},
			Recipient:  ids.From[ids.UserKind](seat),
		}
		_, _, ok, err := refreshApprovalNotice(a.e.As(seat, nil, integration.AdminPerms), a.e.Pool, attempt, clocktest.Now(t), forSingleEmail)
		if err != nil {
			t.Fatalf("checking the notice for %s: %v", seat, err)
		}
		return ok
	}
	if !current(a.e.AdminUser) {
		t.Fatal("the decider's own notice was dropped")
	}

	moveDecider(t, approvalID, a.e.Rep2)

	if current(a.e.AdminUser) {
		t.Error("a notice for a colleague the request no longer names is still sent")
	}
	if !current(a.e.Rep2) {
		t.Error("the new decider's notice was dropped")
	}
}
