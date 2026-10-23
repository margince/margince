// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMailboxDomainReviewRemainsOptionalInformation(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	old := at.AddDate(0, -4, 0)
	mail, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Direction: new("inbound"), Subject: new("Company information"), OccurredAt: &old, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Contacts.EnsureCounterparty(e.Admin(), contacts.EnsureCounterpartyInput{
		Email: "sender@probe.test", DisplayName: "Review sender", Domain: "probe.test", OwnerID: e.AdminUser,
		ActivityID: ids.From[ids.ActivityKind](ids.UUID(mail.Id)), Source: "manual", CapturedBy: "human:" + e.AdminUser.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Contacts.WithholdForwardedDomain(e.Admin(), contacts.DomainForwardEvidence{RequestedDomain: "probe.test", EffectiveURL: "https://other.test"}); err != nil {
		t.Fatal(err)
	}
	questions, _, err := e.Contacts.DomainQuestionsForOwner(e.Admin(), "", 100)
	if err != nil || len(questions) != 1 || questions[0].Domain != "probe.test" {
		t.Fatalf("review access=%+v, %v", questions, err)
	}
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at })
	work, err := feed.Worklist(e.Admin(), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(work.Queue) != 0 || work.Summary.Total != 0 || work.Focus == nil || len(work.Focus.Items) != 0 {
		t.Fatalf("mailbox context generated personal duties: %+v", work)
	}
}
