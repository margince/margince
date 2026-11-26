// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"context"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A promise task card asks on the page of the recipient our mail reached. It
// asks the question every surface asks of the task, and on no other page.
func TestATaskCardAsksOnlyOnThePageOfTheRecipientTheMailReached(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	task, source := ids.NewV7(), ids.NewV7()
	page := &crmcontracts.Contact360{NextSteps: timelineOf(crmcontracts.Activity{
		Id: openapi_types.UUID(task), Kind: "task", Subject: new("Send the listing details"), OccurredAt: now.AddDate(0, 0, -7),
		DueAt: new(now.AddDate(0, 0, 2)), SourceSystem: new(activities.CommitmentTaskSource), SourceActivityId: new(openapi_types.UUID(source)),
	})}
	said := momentCopyFor("en")
	var card crmcontracts.ContactMoment
	var ok bool
	for _, rung := range said.ladder() {
		if card, ok = rung(context.Background(), now, page); ok && card.ClaimKey == openTaskKey {
			break
		}
	}
	if !ok || card.ClaimKey != openTaskKey {
		t.Fatalf("the page drew no task card: %+v", card)
	}
	reached, other := ids.NewV7(), ids.NewV7()
	asked := momentaction.TaskAsked{
		Contact: reached, Sent: activities.PromiseFollowUp{EmailID: ids.NewV7(), At: now.AddDate(0, 0, -1), Precision: "instant"},
		Key: momentaction.DoneKey(openapi_types.UUID(task)), Fingerprint: "fp:task",
	}
	tasks := map[ids.UUID]momentaction.TaskAsked{task: asked}

	question, ok := said.mayBeDone(card, page, doneAsks{contact: ids.From[ids.ContactKind](reached), tasks: tasks})
	if !ok || question.MayBeDone == nil || question.ClaimKey != asked.Key || question.EvidenceFingerprint != asked.Fingerprint ||
		ids.UUID(question.MayBeDone.EmailActivityId) != asked.Sent.EmailID || question.MayBeDone.PromiseType != crmcontracts.ContactMomentMayBeDonePromiseTypeTask {
		t.Fatalf("the reached recipient's page did not ask the task's question: %+v", question)
	}
	if question, ok := said.mayBeDone(card, page, doneAsks{contact: ids.From[ids.ContactKind](other), tasks: tasks}); ok {
		t.Errorf("a page the mail did not reach asked: %+v", question)
	}
	if question, ok := said.mayBeDone(card, page, doneAsks{contact: ids.From[ids.ContactKind](reached)}); ok {
		t.Errorf("a task the shared engine asked nothing of was asked: %+v", question)
	}
}
