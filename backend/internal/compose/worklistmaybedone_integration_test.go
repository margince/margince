// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A promise task asks on the Worklist the question the contact page asks.
// Our later mail on its thread to the recipient may have kept it. Both read
// one follow-up rule and one Not yet, so the reader answers once.

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// promiseTask files the task signal extraction writes for the promise. It
// carries the commitment source and the promise mail as its source, and the
// claim on the recipient points at it.
func promiseTask(t *testing.T, e *integration.Env, p promiseThread) ids.UUID {
	t.Helper()
	promise := ids.UUID(p.claim.SourceActivityId)
	due := p.sentAt.AddDate(0, 0, 2)
	task, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: new("Send the listing details"), Source: "manual",
		AssigneeID: new(ids.From[ids.UserKind](e.AdminUser)), DueAt: &due,
		SourceSystem: new(activities.CommitmentTaskSource), SourceID: new(ids.NewV7().String()), SourceActivityID: &promise,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: p.contact.UUID}},
	})
	if err != nil {
		t.Fatalf("filing the promise task: %v", err)
	}
	e.WsExec(t, `UPDATE conversation_claim SET task_activity_id = $1 WHERE id = $2`, ids.UUID(task.Id), ids.UUID(p.claim.Id))
	return ids.UUID(task.Id)
}

// worklistQuestion reads the reader's Worklist as of then and answers the
// task row's question, nil when the row asks none.
func worklistQuestion(t *testing.T, e *integration.Env, task ids.UUID, at time.Time) *crmcontracts.WorklistMayBeDone {
	t.Helper()
	service := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at })
	day, err := service.Worklist(e.Admin(), "mine", "", ids.UUID{}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range day.Queue {
		if row.Source == "task" && row.Id == task.String() {
			return row.MayBeDone
		}
	}
	t.Fatalf("the promise task is not on the Worklist")
	return nil
}

func TestTheWorklistAsksDoneAsTheContactPageDoesAndHoldsItsNotYet(t *testing.T) {
	e := integration.Setup(t)
	p := seedPromiseThread(t, e)
	task := promiseTask(t, e, p)
	at := p.sentAt.AddDate(0, 0, 7)
	if q := worklistQuestion(t, e, task, at); q != nil {
		t.Fatalf("the Worklist asked before any later mail: %+v", q)
	}

	reply := logThreadMail(e.Admin(), t, e, p.contact, "thread-listing", p.address, "outbound", p.sentAt.AddDate(0, 0, 6))
	q := worklistQuestion(t, e, task, at)
	if q == nil {
		t.Fatal("our later reply on the promise thread asked nothing on the Worklist")
	}
	if ids.UUID(q.EmailActivityId) != reply || ids.UUID(q.ContactId) != p.contact.UUID || q.ClaimKey != momentaction.DoneKey(openapi_types.UUID(task)) {
		t.Fatalf("may_be_done = %+v, want the reply, the recipient and the shared key", q)
	}
	if done := readTask(t, e, task).IsDone; done != nil && *done {
		t.Fatal("the question closed the task by itself")
	}

	// The contact page asks the same question with the same key and fingerprint.
	page := pageAsOf(e, at)
	moment := momentOf(e.Admin(), t, page, p.contact)
	if moment.MayBeDone == nil || moment.MayBeDone.PromiseType != crmcontracts.ContactMomentMayBeDonePromiseTypeTask {
		t.Fatalf("the contact page does not ask about the task: %+v", moment)
	}
	if moment.ClaimKey != q.ClaimKey || moment.EvidenceFingerprint != q.EvidenceFingerprint {
		t.Errorf("the contact page asks under %s/%s, the Worklist under %s/%s",
			moment.ClaimKey, moment.EvidenceFingerprint, q.ClaimKey, q.EvidenceFingerprint)
	}

	// Not yet, given through the contact page's own set-aside.
	if err := page.DismissMoment(e.Admin(), p.contact, crmcontracts.DismissContactMomentRequest{
		ClaimKey: q.ClaimKey, EvidenceFingerprint: q.EvidenceFingerprint,
	}); err != nil {
		t.Fatal(err)
	}
	if again := worklistQuestion(t, e, task, at); again != nil {
		t.Errorf("Not yet did not hold on the Worklist: %+v", again)
	}
	if after := momentOf(e.Admin(), t, page, p.contact); after.MayBeDone != nil {
		t.Errorf("Not yet did not hold on the contact page: %+v", after)
	}

	// A newer mail asks again.
	newer := logThreadMail(e.Admin(), t, e, p.contact, "thread-listing", p.address, "outbound", p.sentAt.AddDate(0, 0, 6).Add(3*time.Hour))
	if again := worklistQuestion(t, e, task, at); again == nil || ids.UUID(again.EmailActivityId) != newer {
		t.Errorf("a newer mail did not ask again: %+v", again)
	}
}
