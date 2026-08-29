// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The bounces lane: the reader's own sends that never arrived, with `open`
// exactly where a person to open exists.

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubBounces struct {
	rows  []BouncedSend
	err   error
	since time.Time
}

func (s *stubBounces) HardBounces(_ context.Context, since time.Time, _ int) ([]BouncedSend, error) {
	s.since = since
	return s.rows, s.err
}

func bounceLaneService(bounces Bounces) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, bounces, nil, nil, nil, fixedClock)
}

func TestABouncedSendNamesItselfAndOpensThePerson(t *testing.T) {
	person := ids.NewV7()
	reported := readInstant.Add(-3 * time.Hour)
	stub := &stubBounces{rows: []BouncedSend{
		{ID: ids.NewV7(), Subject: "Proposal for Weber GmbH", Reason: "550 5.1.1 user unknown", BouncedAt: reported, PersonID: person},
		{ID: ids.NewV7(), Subject: "Intro", BouncedAt: reported},
	}}
	out, err := bounceLaneService(stub).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Bounces == nil || len(*out.Bounces) != 2 {
		t.Fatalf("bounces carries %v, want two sends", out.Bounces)
	}
	// The window is a week of the feed's own clock: a dead address needs
	// fixing whenever the rep next sits down.
	if want := readInstant.Add(-7 * 24 * time.Hour); !stub.since.Equal(want) {
		t.Errorf("window opens at %v, want %v", stub.since, want)
	}
	filed := (*out.Bounces)[0]
	if filed.Title == nil || *filed.Title != "Proposal for Weber GmbH" {
		t.Errorf("title = %v, want the send's own subject line", filed.Title)
	}
	if filed.Detail == nil || *filed.Detail != "550 5.1.1 user unknown" {
		t.Errorf("detail = %v, want the receiving side's own words", filed.Detail)
	}
	if filed.Subject == nil || ids.UUID(filed.Subject.Id) != person {
		t.Fatalf("subject = %v, want the person the send is filed under", filed.Subject)
	}
	if !slices.Contains(filed.Actions, crmcontracts.AttentionItemActions("open")) {
		t.Error("a send filed under a person offers no open — the page where fixing the address lives")
	}
	unfiled := (*out.Bounces)[1]
	if unfiled.Subject != nil || len(unfiled.Actions) != 0 {
		t.Errorf("a send filed under nobody must carry no subject and no verbs, got %v / %v", unfiled.Subject, unfiled.Actions)
	}
}

func TestARefusedBounceReadIsNamedAsWithheld(t *testing.T) {
	out, err := bounceLaneService(&stubBounces{err: apperrors.ErrPermissionDenied}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Bounces != nil {
		t.Fatalf("bounces = %v, want the lane withheld", out.Bounces)
	}
	if out.LanesOmitted == nil || !slices.Contains(*out.LanesOmitted, crmcontracts.AttentionLanesOmitted("bounces")) {
		t.Fatal("a refused bounce read was not named in lanes_omitted")
	}
}

// Every send arriving is an EMPTY lane — the feed looked — which the absent
// lane never promises.
func TestEveryDeliveredSendReadsAsAnEmptyLane(t *testing.T) {
	out, err := bounceLaneService(&stubBounces{}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Bounces == nil || len(*out.Bounces) != 0 {
		t.Fatalf("bounces = %v, want an empty lane", out.Bounces)
	}
}
