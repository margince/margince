// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The undelivered lane: the reader's own sends that were given up on, beside
// the bounce lane that carries the ones which arrived and were refused.

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubUndelivered struct {
	rows  []ParkedSend
	err   error
	since time.Time
}

func (s *stubUndelivered) ParkedSends(_ context.Context, since time.Time, _ int) ([]ParkedSend, error) {
	s.since = since
	return s.rows, s.err
}

func undeliveredLaneService(undelivered Undelivered) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock,
	).WithUndelivered(undelivered)
}

func TestAGivenUpSendNamesItselfAndOpensThePerson(t *testing.T) {
	person := ids.NewV7()
	abandoned := readInstant.Add(-3 * time.Hour)
	stub := &stubUndelivered{rows: []ParkedSend{
		{ID: ids.NewV7(), Subject: "Proposal for Weber GmbH", Reason: "the mailbox is no longer send-capable", ParkedAt: abandoned, PersonID: person},
		{ID: ids.NewV7(), Subject: "Intro", ParkedAt: abandoned},
	}}
	out, err := undeliveredLaneService(stub).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Undelivered == nil || len(*out.Undelivered) != 2 {
		t.Fatalf("undelivered carries %v, want two sends", out.Undelivered)
	}
	// The bounce lane's week: a message nobody sent is not less urgent for
	// being a day older.
	if want := readInstant.Add(-7 * 24 * time.Hour); !stub.since.Equal(want) {
		t.Errorf("window opens at %v, want %v", stub.since, want)
	}
	filed := (*out.Undelivered)[0]
	if filed.Source != crmcontracts.AttentionItemSource("undelivered") {
		t.Errorf("source = %q, want undelivered — a send that never left is not a bounce", filed.Source)
	}
	if filed.Title == nil || *filed.Title != "Proposal for Weber GmbH" {
		t.Errorf("title = %v, want the send's own subject line", filed.Title)
	}
	if filed.Detail == nil || *filed.Detail != "the mailbox is no longer send-capable" {
		t.Errorf("detail = %v, want the dispatcher's own words", filed.Detail)
	}
	if filed.Subject == nil || ids.UUID(filed.Subject.Id) != person {
		t.Fatalf("subject = %v, want the person the send is filed under", filed.Subject)
	}
	if !slices.Contains(filed.Actions, crmcontracts.AttentionItemActions("open")) {
		t.Error("a send filed under a person offers no open — the page where sending it again lives")
	}
	unfiled := (*out.Undelivered)[1]
	if unfiled.Subject != nil || len(unfiled.Actions) != 0 {
		t.Errorf("a send filed under nobody must carry no subject and no verbs, got %v / %v", unfiled.Subject, unfiled.Actions)
	}
}

func TestARefusedUndeliveredReadIsNamedAsWithheld(t *testing.T) {
	out, err := undeliveredLaneService(&stubUndelivered{err: apperrors.ErrPermissionDenied}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Undelivered != nil {
		t.Fatalf("undelivered = %v, want the lane withheld", out.Undelivered)
	}
	if out.LanesOmitted == nil || !slices.Contains(*out.LanesOmitted, crmcontracts.AttentionLanesOmitted("undelivered")) {
		t.Fatal("a refused undelivered read was not named in lanes_omitted")
	}
}

// Every send leaving is an EMPTY lane — the feed looked — which the absent
// lane never promises.
func TestEverySendThatLeftReadsAsAnEmptyLane(t *testing.T) {
	out, err := undeliveredLaneService(&stubUndelivered{}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Undelivered == nil || len(*out.Undelivered) != 0 {
		t.Fatalf("undelivered = %v, want an empty lane", out.Undelivered)
	}
}
