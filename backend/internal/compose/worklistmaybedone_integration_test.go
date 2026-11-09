// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

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

func TestTaskReadsOfferNoCompletionConfirmation(t *testing.T) {
	e := integration.Setup(t)
	p := seedPromiseThread(t, e)
	task := promiseTask(t, e, p)
	logThreadMail(e.Admin(), t, e, p.contact, "thread-listing", p.address, "outbound", p.sentAt.AddDate(0, 0, 6))
	at := p.sentAt.AddDate(0, 0, 7)
	service := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at })
	for _, ctx := range []context.Context{e.Admin(), e.AgentFor(t, e.AdminUser, nil, integration.AdminPerms)} {
		day, err := service.Worklist(ctx, "mine", "", ids.UUID{}, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range day.Queue {
			if row.Source == "task" && row.Id == task.String() {
				found = true
				if row.MayBeDone != nil {
					t.Fatalf("task asks for confirmation: %+v", row.MayBeDone)
				}
			}
		}
		if !found {
			t.Fatal("task is missing")
		}
	}
	company := e.SeedCompany(t, "Kunde GmbH", nil)
	e.WsExec(t, `INSERT INTO relationship(kind,contact_id,company_id,source,captured_by) VALUES('employment',$1,$2,'manual','human:x')`, p.contact.UUID, company)
	page, err := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, approvals.NewService(e.DB()), func() time.Time { return at }).Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, step := range page.NextSteps.Data {
		if ids.UUID(step.ActivityId) == task {
			found = true
			if step.MayBeDone != nil {
				t.Fatalf("company task asks for confirmation: %+v", step.MayBeDone)
			}
		}
	}
	if !found {
		t.Fatal("company task is missing")
	}
	if done := readTask(t, e, task).IsDone; done != nil && *done {
		t.Fatal("reading a task completed it")
	}
}
