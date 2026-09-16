// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The contact pass: whose row it is, and which side wrote last.

import (
	"context"
	"errors"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubContactTouch struct {
	moments map[ids.UUID]TouchMoments
	asked   [][]ids.UUID
	err     error
}

func (s *stubContactTouch) LastTouch(_ context.Context, contactIDs []ids.UUID) (map[ids.UUID]TouchMoments, error) {
	s.asked = append(s.asked, append([]ids.UUID(nil), contactIDs...))
	return s.moments, s.err
}

func contactRow(contact ids.UUID, label *string) crmcontracts.WorklistItem {
	return crmcontracts.WorklistItem{
		Id:      ids.NewV7().String(),
		Source:  "task",
		Contact: &crmcontracts.WorklistContactFacts{Id: openapi_types.UUID(contact), Label: label},
	}
}

// A waiting message filed under a deal is still a message from a person: the
// subject is the deal and the contact is the sender, and both travel.
func TestAWaitingRowNamesItsSenderBesideTheDealItIsFiledUnder(t *testing.T) {
	contact, deal := ids.NewV7(), ids.NewV7()
	row := classifyWaiting(WaitingCustomer{
		ActivityID: ids.NewV7(), Since: rankInstant.Add(-48 * time.Hour),
		ContactID: contact, DealID: deal, HasOpenDeal: true,
	}, rankInstant).item
	if row.Subject == nil || row.Subject.Type != subjectDeal {
		t.Fatalf("the row's subject is %+v, wanted the deal the thread belongs to", row.Subject)
	}
	if row.Contact == nil || ids.UUID(row.Contact.Id) != contact {
		t.Fatalf("the row's contact is %+v, wanted the sender %s", row.Contact, contact)
	}
	// A stranger's message names nobody rather than a zero id.
	stranger := classifyWaiting(WaitingCustomer{ActivityID: ids.NewV7(), Since: rankInstant}, rankInstant).item
	if stranger.Contact != nil {
		t.Fatalf("a stranger's message names %+v, wanted nobody", stranger.Contact)
	}
}

// A lane item whose subject is a person, and a meeting read on a person's
// page, both name that person — with the subject's label where it has one.
func TestALaneItemNamesThePersonItIsAbout(t *testing.T) {
	contact := ids.NewV7()
	name := "Sonya Beck"
	about := crmcontracts.AttentionItem{Subject: &crmcontracts.AttentionSubject{
		Type: subjectContact, Id: openapi_types.UUID(contact), Label: &name,
	}}
	if got := contactOf(about); got == nil || ids.UUID(got.Id) != contact || got.Label == nil || *got.Label != name {
		t.Fatalf("a row about %s carries %+v", name, got)
	}
	attendee := openapi_types.UUID(contact)
	meeting := crmcontracts.AttentionItem{
		Subject:     &crmcontracts.AttentionSubject{Type: "activity", Id: openapi_types.UUID(ids.NewV7())},
		WithContact: &attendee,
	}
	if got := contactOf(meeting); got == nil || ids.UUID(got.Id) != contact {
		t.Fatalf("a meeting with %s carries %+v", contact, got)
	}
	deal := crmcontracts.AttentionItem{Subject: &crmcontracts.AttentionSubject{Type: subjectDeal, Id: openapi_types.UUID(ids.NewV7())}}
	if got := contactOf(deal); got != nil {
		t.Fatalf("a deal row names %+v, wanted nobody", got)
	}
}

// Every contact on the page is asked about once, and each row gets its own
// two moments; a contact the reader may not see keeps its id and gains none.
func TestTheRowsSayWhenEachSideLastWrote(t *testing.T) {
	sonya, hidden := ids.NewV7(), ids.NewV7()
	theyWrote := rankInstant.Add(-3 * 24 * time.Hour)
	touch := &stubContactTouch{moments: map[ids.UUID]TouchMoments{
		sonya: {LastInbound: &theyWrote},
	}}
	svc := (&Service{}).WithContactTouch(touch)
	rows := []crmcontracts.WorklistItem{
		contactRow(sonya, nil), contactRow(sonya, nil), contactRow(hidden, nil),
		{Id: "deal", Source: "deal_at_risk"},
	}

	if err := svc.nameTheContacts(context.Background(), rows); err != nil {
		t.Fatalf("naming the contacts: %v", err)
	}

	if len(touch.asked) != 1 || len(touch.asked[0]) != 2 {
		t.Fatalf("the reader was asked %v, wanted one call naming each contact once", touch.asked)
	}
	for _, row := range rows[:2] {
		got := row.Contact.Touch
		if got == nil || got.LastInboundAt == nil || !got.LastInboundAt.Equal(theyWrote) || got.LastOutboundAt != nil {
			t.Fatalf("Sonya's row says %+v, wanted she wrote at %s and we never did", got, theyWrote)
		}
	}
	if rows[2].Contact == nil || rows[2].Contact.Touch != nil {
		t.Fatalf("the hidden contact's row carries %+v, wanted the id and no moments", rows[2].Contact)
	}
}

// A reader who may not read activity is refused the moments, not the page:
// every row keeps its contact and none claims a date.
func TestARefusedActivityReadWithholdsTheMomentsNotTheRows(t *testing.T) {
	svc := (&Service{}).WithContactTouch(&stubContactTouch{err: apperrors.ErrPermissionDenied})
	rows := []crmcontracts.WorklistItem{contactRow(ids.NewV7(), nil)}
	if err := svc.nameTheContacts(context.Background(), rows); err != nil {
		t.Fatalf("a refusal failed the page: %v", err)
	}
	if rows[0].Contact == nil || rows[0].Contact.Touch != nil {
		t.Fatalf("the row carries %+v, wanted its contact and no moments", rows[0].Contact)
	}
	// Any other failure is the reader's to see, never rendered as silence.
	broken := (&Service{}).WithContactTouch(&stubContactTouch{err: errors.New("connection reset")})
	if err := broken.nameTheContacts(context.Background(), rows); err == nil {
		t.Fatal("a database failure was rendered as a contact nobody wrote to")
	}
}

// The projection carries some contacts as an id alone — the sender behind a
// deal-filed thread — and the pass names them in one read, leaving the ones
// the subject already named alone.
func TestUnnamedContactsAreNamedOnceUnderTheReadersGrants(t *testing.T) {
	sonya, hidden, named := ids.NewV7(), ids.NewV7(), ids.NewV7()
	already := "Named By The Lane"
	names := &stubNames{labels: map[ids.UUID]string{sonya: "Sonya Beck"}}
	svc := (&Service{names: names}).WithContactTouch(&stubContactTouch{})
	rows := []crmcontracts.WorklistItem{
		contactRow(sonya, nil), contactRow(sonya, nil), contactRow(hidden, nil), contactRow(named, &already),
	}

	if err := svc.nameTheContacts(context.Background(), rows); err != nil {
		t.Fatalf("naming the contacts: %v", err)
	}

	if len(names.calls) != 1 || len(names.calls[0]) != 2 {
		t.Fatalf("the resolver was asked %v, wanted one call for the two unnamed contacts", names.calls)
	}
	if rows[0].Contact.Label == nil || *rows[0].Contact.Label != "Sonya Beck" {
		t.Fatalf("Sonya's row is labelled %v", rows[0].Contact.Label)
	}
	if rows[2].Contact.Label != nil {
		t.Fatalf("a contact the reader may not see is labelled %q", *rows[2].Contact.Label)
	}
	if *rows[3].Contact.Label != already {
		t.Fatalf("the lane's own label was replaced with %q", *rows[3].Contact.Label)
	}
}

// Unbound, the pass changes nothing: rows name their contact and no moments,
// which is the shape every row had before the seam.
func TestAnUnboundReaderLeavesTheContactsAsTheLaneNamedThem(t *testing.T) {
	rows := []crmcontracts.WorklistItem{contactRow(ids.NewV7(), nil)}
	if err := (&Service{}).nameTheContacts(context.Background(), rows); err != nil {
		t.Fatalf("an unbound pass failed: %v", err)
	}
	if rows[0].Contact.Touch != nil || rows[0].Contact.Label != nil {
		t.Fatalf("an unbound pass invented %+v", rows[0].Contact)
	}
}
