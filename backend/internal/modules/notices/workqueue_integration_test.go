// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestStageHistoryStaysInTheBellWithoutBecomingOutstandingWork(t *testing.T) {
	e := setupNotices(t)
	useful, err := e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: "lead_sla", Subject: "A lead needs a reply"})
	if err != nil {
		t.Fatal(err)
	}
	otherNotice, err := e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: "automation", Subject: "Contact review needed", Target: Target{Type: "contact", ID: ids.NewV7()}, DedupeKey: "stage_change_notify:" + ids.NewV7().String()})
	if err != nil {
		t.Fatal(err)
	}
	var stage ids.UUID
	for i := range 3 {
		notice := NewNotice{Recipient: e.recipient, Kind: "automation", Subject: "Qualified to Won", Target: Target{Type: "deal", ID: ids.NewV7()}, DedupeKey: "stage_change_notify:" + ids.NewV7().String()}
		if i == 0 {
			notice.DedupeKey = ""
			notice.Origin = &crmcontracts.NoticeOrigin{ActorType: "system", ActorId: "system:stage", EventId: openapi_types.UUID(ids.NewV7()), OccurredAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
			notice.Origin.StageChange = &struct {
				FromName *string `json:"from_name,omitempty"`
				ToName   *string `json:"to_name,omitempty"`
			}{}
		}
		stage, err = e.store.Create(e.engineCtx(), notice)
		if err != nil {
			t.Fatal(err)
		}
	}
	reader := e.asUser(e.recipient)
	work, err := e.store.UnreadWorkFor(reader, 1)
	if err != nil || len(work) != 1 || work[0].ID != otherNotice {
		t.Fatalf("stage history consumed work page: %+v, %v", work, err)
	}
	allWork, err := e.store.UnreadWorkFor(reader, 10)
	if err != nil || len(allWork) != 2 || allWork[1].ID != useful {
		t.Fatalf("actionable lead lost: %+v, %v", allWork, err)
	}
	bell, err := e.store.ListFor(reader, 10, "")
	if err != nil || len(bell.Items) != 5 || bell.UnreadCount != 5 {
		t.Fatalf("bell history was suppressed or acknowledged: %+v, %v", bell, err)
	}
	unread, err := e.store.UnreadFor(reader, 1)
	if err != nil || len(unread) != 1 || unread[0].ID != stage {
		t.Fatalf("ordinary notice read changed: %+v, %v", unread, err)
	}
	other, err := e.store.UnreadWorkFor(e.asUser(e.other), 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("work notice crossed recipients: %+v, %v", other, err)
	}
}

func TestApprovalNotificationsStayInTheBellWithoutDuplicatingDecisions(t *testing.T) {
	e := setupNotices(t)
	useful, err := e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: "lead_sla", Subject: "A lead needs a reply"})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{ApprovalNoticeKey(ids.NewV7()), ""} {
		_, err = e.store.Create(e.engineCtx(), NewNotice{Recipient: e.recipient, Kind: KindApprovalPending, Subject: "Review a proposed change", Target: Target{Type: "deal", ID: ids.NewV7()}, DedupeKey: key})
		if err != nil {
			t.Fatal(err)
		}
	}
	reader := e.asUser(e.recipient)
	work, err := e.store.UnreadWorkFor(reader, 1)
	if err != nil || len(work) != 1 || work[0].ID != useful {
		t.Fatalf("approval mirrors consumed work page: %+v, %v", work, err)
	}
	bell, err := e.store.ListFor(reader, 10, "")
	if err != nil || len(bell.Items) != 3 || bell.UnreadCount != 3 {
		t.Fatalf("bell notifications were suppressed or acknowledged: %+v, %v", bell, err)
	}
	unread, err := e.store.UnreadFor(reader, 10)
	if err != nil || len(unread) != 3 || unread[0].Kind != KindApprovalPending {
		t.Fatalf("ordinary notice read changed: %+v, %v", unread, err)
	}
}
