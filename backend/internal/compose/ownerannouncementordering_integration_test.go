// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAQueuedOwnerAnnouncementRechecksTheRecipientsRole(t *testing.T) {
	e := setupReconcile(t)
	manager := memberManagerFor(t, e.Env)
	deal, id := e.stagedFollowUp(t, "A queued request")
	e.tellPreviousOwner(t, e.Rep1, id)
	e.reassignAsTheProduct(t, deal, &e.Rep2)
	if err := manager.service.ChangeUserRole(e.Admin(), recoveryAdminIdentity(e.Env), ids.From[ids.UserKind](e.Rep2), "read_only"); err != nil {
		t.Fatal(err)
	}
	e.workOwnerNotices(t)
	if n := e.liveLines(t, e.Rep2, id); n != 0 {
		t.Fatalf("revoked recipient received %d notices", n)
	}
	if n := e.mailJobsFor(t, e.Rep2); n != 0 {
		t.Fatalf("revoked recipient received %d mail jobs", n)
	}
	if n := e.liveLines(t, e.Rep1, id); n != 0 {
		t.Fatalf("previous owner kept %d notices", n)
	}
}

func TestARolledBackOwnerChangeQueuesNoAnnouncement(t *testing.T) {
	e := setupReconcile(t)
	memberManagerFor(t, e.Env)
	deal, id := e.stagedFollowUp(t, "An owner change rolled back")
	e.tellPreviousOwner(t, e.Rep1, id)
	failed := errors.New("the caller rejected the complete transaction")
	err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		next := ids.From[ids.UserKind](e.Rep2)
		store := deals.NewStore(e.DB(), DealsInstallation())
		if _, err := store.UpdateDealTx(e.Admin(), tx, ids.From[ids.DealKind](deal), deals.UpdateDealInput{OwnerID: &next}, deals.CustomColumns{}); err != nil {
			return err
		}
		return failed
	})
	if !errors.Is(err, failed) {
		t.Fatalf("owner-change rollback: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM river_job WHERE kind='approval_owner_notice'`); n != 0 {
		t.Fatalf("rollback left %d owner announcement jobs", n)
	}
	if n := e.liveLines(t, e.Rep1, id); n != 1 {
		t.Fatalf("rollback lost the previous owner's notice: %d", n)
	}
	var owner ids.UUID
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT owner_id FROM deal WHERE id=$1`, deal).Scan(&owner)
	}); err != nil {
		t.Fatal(err)
	}
	if owner != e.Rep1 {
		t.Fatal("rollback changed the deal's owner")
	}
}

func TestARunningAnnouncementDoesNotAbsorbALaterReturnToTheSameOwner(t *testing.T) {
	e := setupReconcile(t)
	memberManagerFor(t, e.Env)
	deal, id := e.stagedFollowUp(t, "Return while an announcement completes")
	e.tellPreviousOwner(t, e.Rep1, id)
	e.reassignAsTheProduct(t, deal, &e.Rep2)
	e.workOwnerNotices(t)
	e.reassignAsTheProduct(t, deal, &e.Rep1)
	now := clocktest.Now(t)
	rows, err := riverpgxv5.New(e.Pool).GetExecutor().JobGetAvailable(t.Context(), &riverdriver.JobGetAvailableParams{
		ClientID: "owner-announcement-test", MaxAttemptedBy: 10, MaxToLock: 100, Now: &now, Queue: "default", Schema: "public",
	})
	if err != nil {
		t.Fatal(err)
	}
	worked := false
	production := &approvalOwnerNoticeWorker{pool: e.Pool, now: func() time.Time { return now }}
	for _, row := range rows {
		if row.Kind != (ApprovalOwnerNoticeArgs{}).Kind() {
			continue
		}
		var args ApprovalOwnerNoticeArgs
		if err := json.Unmarshal(row.EncodedArgs, &args); err != nil {
			t.Fatal(err)
		}
		if args.ApprovalID != id.UUID || args.DeciderID != e.Rep1 {
			continue
		}
		if err := production.Work(t.Context(), &river.Job[ApprovalOwnerNoticeArgs]{JobRow: row, Args: args}); err != nil {
			t.Fatal(err)
		}
		worked = true
	}
	if !worked {
		t.Fatal("the earlier announcement never entered its running state")
	}
	if n := e.liveLines(t, e.Rep1, id); n != 1 {
		t.Fatalf("the running announcement wrote %d notices", n)
	}
	// River completion follows the worker's commit, so a new handover can arrive in between.
	e.reassignAsTheProduct(t, deal, &e.Rep2)
	e.reassignAsTheProduct(t, deal, &e.Rep1)
	e.workOwnerNotices(t)
	if n := e.liveLines(t, e.Rep1, id); n != 1 {
		t.Fatalf("the earlier running job absorbed the later announcement: %d notices", n)
	}
	if n := e.liveLines(t, e.Rep2, id); n != 0 {
		t.Fatalf("the former owner kept %d notices", n)
	}
}

func TestAnOwnerAnnouncementChecksExpiryAfterWaitingForTheRequest(t *testing.T) {
	e := setupReconcile(t)
	memberManagerFor(t, e.Env)
	deal, id := e.stagedFollowUp(t, "Expires during the announcement wait")
	e.reassignAsTheProduct(t, deal, &e.Rep2)
	var at atomic.Int64
	at.Store(clocktest.Now(t).UnixNano())
	worker := approvalOwnerNoticeWorker{pool: e.Pool, now: func() time.Time { return time.Unix(0, at.Load()) }}
	done := make(chan error, 1)
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		var expires time.Time
		if err := tx.QueryRow(e.Admin(), `SELECT expires_at FROM approval WHERE id=$1 FOR UPDATE`, id).Scan(&expires); err != nil {
			return err
		}
		go func() {
			done <- worker.Work(t.Context(), &river.Job[ApprovalOwnerNoticeArgs]{Args: ApprovalOwnerNoticeArgs{Workspace: e.WS, ApprovalID: id.UUID, DeciderID: e.Rep2}})
		}()
		waitForBackendBlockedBy(t, tx, backendPID(t, tx), done)
		at.Store(expires.Add(time.Second).UnixNano())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if n := e.liveLines(t, e.Rep2, id); n != 0 {
		t.Fatalf("expired request produced %d notices", n)
	}
	if n := e.mailJobsFor(t, e.Rep2); n != 0 {
		t.Fatalf("expired request produced %d mail jobs", n)
	}
}

func TestACommittedOwnerChangeQueuesTheAnnouncementInTheDealTransaction(t *testing.T) {
	e := setupReconcile(t)
	memberManagerFor(t, e.Env)
	deal, id := e.stagedFollowUp(t, "A committed owner change")
	e.tellPreviousOwner(t, e.Rep1, id)
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		next := ids.From[ids.UserKind](e.Rep2)
		_, err := deals.NewStore(e.DB(), DealsInstallation()).UpdateDealTx(e.Admin(), tx, ids.From[ids.DealKind](deal), deals.UpdateDealInput{OwnerID: &next}, deals.CustomColumns{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM river_job WHERE kind='approval_owner_notice' AND max_attempts > 3`); n != 1 {
		t.Fatalf("commit produced %d jobs with the normal retry ladder", n)
	}
	if n := e.liveLines(t, e.Rep1, id); n != 0 {
		t.Fatalf("previous owner kept %d notices", n)
	}
	e.workOwnerNotices(t)
	if n := e.liveLines(t, e.Rep2, id); n != 1 {
		t.Fatalf("new owner received %d notices", n)
	}
}

func TestAnOwnerAnnouncementHoldsAuthorityWhileWaitingForTheRequest(t *testing.T) {
	e := setupReconcile(t)
	manager := memberManagerFor(t, e.Env)
	deal, id := e.stagedFollowUp(t, "Authority remains held while announcing")
	e.reassignAsTheProduct(t, deal, &e.Rep2)
	worker := approvalOwnerNoticeWorker{pool: e.Pool, now: func() time.Time { return clocktest.Now(t) }}
	noticeDone, roleDone := make(chan error, 1), make(chan error, 1)
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		var locked ids.UUID
		if err := tx.QueryRow(e.Admin(), `SELECT id FROM approval WHERE id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
			return err
		}
		go func() {
			noticeDone <- worker.Work(t.Context(), &river.Job[ApprovalOwnerNoticeArgs]{Args: ApprovalOwnerNoticeArgs{Workspace: e.WS, ApprovalID: id.UUID, DeciderID: e.Rep2}})
		}()
		waitForBackendBlockedBy(t, tx, backendPID(t, tx), noticeDone)
		go func() {
			roleDone <- manager.service.ChangeUserRole(e.Admin(), recoveryAdminIdentity(e.Env), ids.From[ids.UserKind](e.Rep2), "read_only")
		}()
		waitForBackendsBlockedBy(t, tx, backendPID(t, tx), 2, roleDone)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := <-noticeDone; err != nil {
		t.Fatal(err)
	}
	if err := <-roleDone; err != nil {
		t.Fatal(err)
	}
}

func TestASendReservationYieldsToTheDealWriterWithoutHoldingItsDecision(t *testing.T) {
	e := setupReconcile(t)
	memberManagerFor(t, e.Env)
	deal, id := e.stagedFollowUp(t, "A writer has priority over a send")
	reader := e.As(e.Rep1, nil, integration.AdminPerms)
	basis := readBasis(reader, t, e.Env, e.Rep1, sendRefs{approvals: []ids.UUID{id.UUID}}, clocktest.Now(t))
	if len(basis.rows) == 0 || len(basis.excluded) > 0 {
		t.Fatal("the decision had no qualifying send basis")
	}
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		var locked ids.UUID
		if err := tx.QueryRow(e.Admin(), `SELECT id FROM deal WHERE id=$1 FOR UPDATE`, deal).Scan(&locked); err != nil {
			return err
		}
		done := make(chan error, 1)
		go func() { done <- holdBasis(reader, e.Env, basis, clocktest.Now(t)) }()
		select {
		case err := <-done:
			if !errors.Is(err, errSendBasisChanged) {
				t.Fatalf("busy writer did not invalidate the reservation: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("send waited for the writer while holding its decision")
		}
		next := ids.From[ids.UserKind](e.Rep2)
		_, err := deals.NewStore(e.DB(), DealsInstallation()).UpdateDealTx(e.Admin(), tx, ids.From[ids.DealKind](deal), deals.UpdateDealInput{OwnerID: &next}, deals.CustomColumns{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
