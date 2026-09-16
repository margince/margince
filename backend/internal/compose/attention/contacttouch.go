// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The contact pass: every row that names a human says who, and how the silence
// runs both ways — when they last wrote to us, when we last wrote to them.
//
// A rep answering a row asks whose row it is before how long it has waited,
// and which side wrote last is the whole question: a contact we mailed a
// fortnight ago with no reply and one who wrote this morning have the same
// last-touch date and opposite meanings. The contact's own page answers it;
// before this pass a queue row did not, so a reader opened the record to
// learn what the row should have said.
//
// One read for every contact the page names, after the page is cut, like the
// owner pass beside it: a follow-up read per row is the N+1 this feed exists
// to avoid, and a read over the whole day would price rows the page never
// shows.

import (
	"context"
	"errors"
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ContactTouch answers when each of a set of contacts last wrote to us and
// when we last wrote to them, under the caller's own grants.
//
// One call for every contact on the page, like Names, rather than one per
// row. A contact the caller may not read is simply absent from the answer,
// and the row then names the contact and no moments; a caller who may not
// read activity at all is refused with apperrors.ErrPermissionDenied, and
// every row on the page keeps its contact and no moments — a withheld answer
// is not a contact nobody ever wrote to.
type ContactTouch interface {
	LastTouch(ctx context.Context, contactIDs []ids.UUID) (map[ids.UUID]TouchMoments, error)
}

// TouchMoments is one contact's two directions. Nil means it never happened.
type TouchMoments struct {
	LastInbound  *time.Time
	LastOutbound *time.Time
}

// contactOf names the human behind a lane item: the attendee a meeting's brief
// is read on, else the subject when the subject is a contact. The label
// travels with the subject's, already resolved under the reader's grants, so
// the pass below never asks twice for one name.
func contactOf(item crmcontracts.AttentionItem) *crmcontracts.WorklistContactFacts {
	if item.WithContact != nil {
		return &crmcontracts.WorklistContactFacts{Id: *item.WithContact}
	}
	if item.Subject != nil && item.Subject.Type == subjectContact {
		return &crmcontracts.WorklistContactFacts{Id: item.Subject.Id, Label: item.Subject.Label}
	}
	return nil
}

// waitingContact names the sender of a waiting message, whatever record the
// thread is filed under: the subject may be the deal, the reply goes to a
// contact. Nil for a stranger's message, which names nobody.
func waitingContact(waiting WaitingCustomer) *crmcontracts.WorklistContactFacts {
	if waiting.ContactID.IsZero() {
		return nil
	}
	return &crmcontracts.WorklistContactFacts{Id: openapi_types.UUID(waiting.ContactID)}
}

// nameTheContacts fills the label and the two moments on every row that names
// a contact: one Names read for the contacts the projection left unnamed, one
// ContactTouch read for them all.
func (s *Service) nameTheContacts(ctx context.Context, rows []crmcontracts.WorklistItem) error {
	named := contactsOn(rows)
	if len(named) == 0 {
		return nil
	}
	if err := s.labelContacts(ctx, rows, named); err != nil {
		return err
	}
	if s.contactTouch == nil {
		return nil
	}
	moments, err := s.contactTouch.LastTouch(ctx, named)
	// Refused is withheld, not failed: the reader may not read activity, and
	// the page still stands with every contact named and no moments on any.
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("attention: reading when the contacts on the queue last wrote: %w", err)
	}
	for i := range rows {
		contact := rows[i].Contact
		if contact == nil {
			continue
		}
		touch, known := moments[ids.UUID(contact.Id)]
		if !known {
			continue
		}
		contact.Touch = &crmcontracts.WorklistContactTouch{
			LastInboundAt:  touch.LastInbound,
			LastOutboundAt: touch.LastOutbound,
		}
	}
	return nil
}

// labelContacts names the contacts the projection carried as an id alone —
// the sender of a thread filed under a deal, the attendee of a meeting — in
// one read, under the same resolver that named every subject.
func (s *Service) labelContacts(ctx context.Context, rows []crmcontracts.WorklistItem, named []ids.UUID) error {
	if s.names == nil {
		return nil
	}
	unnamed := make([]ids.UUID, 0, len(named))
	seen := make(map[ids.UUID]bool, len(named))
	for _, row := range rows {
		if row.Contact == nil || row.Contact.Label != nil || seen[ids.UUID(row.Contact.Id)] {
			continue
		}
		seen[ids.UUID(row.Contact.Id)] = true
		unnamed = append(unnamed, ids.UUID(row.Contact.Id))
	}
	if len(unnamed) == 0 {
		return nil
	}
	labels, err := s.names.Labels(ctx, subjectContact, unnamed)
	if err != nil {
		return fmt.Errorf("attention: naming the contacts on the queue: %w", err)
	}
	for i := range rows {
		contact := rows[i].Contact
		if contact == nil || contact.Label != nil {
			continue
		}
		if label, known := labels[ids.UUID(contact.Id)]; known {
			contact.Label = &label
		}
	}
	return nil
}

// contactsOn gathers the contacts a page names, once each, in the order they
// were met — so a reader that bounds its answer drops the same contacts for
// the same page rather than a different set each read.
func contactsOn(rows []crmcontracts.WorklistItem) []ids.UUID {
	var named []ids.UUID
	seen := map[ids.UUID]bool{}
	for _, row := range rows {
		if row.Contact == nil {
			continue
		}
		id := ids.UUID(row.Contact.Id)
		if seen[id] {
			continue
		}
		seen[id] = true
		named = append(named, id)
	}
	return named
}
