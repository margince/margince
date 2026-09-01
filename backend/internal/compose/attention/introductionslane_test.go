// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The introductions lane: the asks waiting on this colleague to answer.
//
// Three properties, and the third is the one worth a test of its own. The card
// names the contact rather than writing an English sentence; the verb is
// `decide` and only that; and the lane is ABSENT when unbound, which is a
// different fact from empty — "this installation does not do introductions" is
// not "nobody has asked you for one", and a colleague told the second when the
// first is true stops looking.

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubIntroductions struct {
	rows []PendingIntroduction
	err  error
}

func (s *stubIntroductions) Pending(context.Context, int) ([]PendingIntroduction, error) {
	return s.rows, s.err
}

func introductionsLaneService(i Introductions) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock,
	).WithIntroductions(i)
}

func TestAnIntroductionAskNamesTheContactAndOffersOneVerb(t *testing.T) {
	contact := ids.NewV7()
	due := readInstant.Add(7 * 24 * time.Hour)
	svc := introductionsLaneService(&stubIntroductions{rows: []PendingIntroduction{
		{
			ID: ids.NewV7(), PersonID: contact,
			Reason:      "Dana reopened the retrofit conversation after 41 days.",
			RequestedAt: readInstant, DueAt: due,
		},
	}})

	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Introductions == nil || len(*out.Introductions) != 1 {
		t.Fatalf("introductions carries %v, want one", out.Introductions)
	}
	ask := (*out.Introductions)[0]

	// The contact rides as a SUBJECT, not as a title. The label is filled in
	// later under the reader's own grants (labels.go), which is why nothing
	// here writes a name — and why nothing here writes a sentence, on a
	// product that ships three languages.
	if ask.Subject == nil || ids.UUID(ask.Subject.Id) != contact {
		t.Errorf("subject = %v, want the contact %v", ask.Subject, contact)
	}
	if ask.Title != nil {
		t.Errorf("title = %v, want none — the headline is the contact's name, "+
			"which the label pass resolves", *ask.Title)
	}
	if ask.Detail == nil ||
		*ask.Detail != "Dana reopened the retrofit conversation after 41 days." {
		t.Errorf("detail = %v, want the requester's own reason verbatim", ask.Detail)
	}
	// The deadline travels: a colleague reading this queue is deciding what to
	// answer first, and the ask about to lapse is the one that matters.
	if ask.DueAt == nil || !ask.DueAt.Equal(due) {
		t.Errorf("due_at = %v, want the ask's own %v", ask.DueAt, due)
	}
	if !slices.Contains(ask.Actions, crmcontracts.AttentionItemActions("decide")) {
		t.Fatal("an ask offers no decide — the one verb a colleague has")
	}
	// And ONLY decide. An ask answered from a queue row would be a colleague's
	// relationship spent on one click, with none of the four answers chosen.
	if len(ask.Actions) != 1 {
		t.Errorf("actions = %v, want decide alone", ask.Actions)
	}
}

func TestARefusedIntroductionsReadIsNamedAsWithheld(t *testing.T) {
	svc := introductionsLaneService(&stubIntroductions{err: apperrors.ErrPermissionDenied})

	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.Introductions != nil {
		t.Errorf("a refused read rendered a lane (%v) rather than withholding it",
			out.Introductions)
	}
	if out.LanesOmitted == nil || !slices.Contains(*out.LanesOmitted,
		crmcontracts.AttentionLanesOmitted("introductions")) {
		t.Errorf("lanes_omitted = %v, want introductions named — a withheld lane "+
			"that is merely absent reads as a clear queue", out.LanesOmitted)
	}
}

// An unbound lane is ABSENT, and a bound-but-empty one is empty. The two are
// different answers and a colleague acts differently on each.
func TestAnUnboundIntroductionsLaneIsAbsentAndAnEmptyOneIsEmpty(t *testing.T) {
	unbound, err := introductionsLaneService(nil).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling without the lane: %v", err)
	}
	if unbound.Introductions != nil {
		t.Errorf("an unbound lane rendered %v; want absent", unbound.Introductions)
	}

	empty, err := introductionsLaneService(&stubIntroductions{}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling with an empty lane: %v", err)
	}
	if empty.Introductions == nil {
		t.Fatal("a bound lane with nothing in it is absent; want an empty lane")
	}
	if len(*empty.Introductions) != 0 {
		t.Errorf("an empty lane carries %v", *empty.Introductions)
	}
	if empty.Counts.Introductions == nil || *empty.Counts.Introductions != 0 {
		t.Errorf("count = %v, want zero — a badge cannot be drawn from a missing number",
			empty.Counts.Introductions)
	}
}
