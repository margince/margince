// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What changes while the model reads: a mailbox withdrawn, a body downgraded,
// or a title a colleague cleared. Each is made through the writer a colleague or the
// product uses, from inside the scripted model call.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// withdrawals are the ways a mail's mailbox or body can stop being readable.
var withdrawals = []string{"the mailbox is switched off", "the mailbox is disconnected", "the body is found incomplete"}

func (h *historyEnv) withdraw(t *testing.T, how string, mail ids.UUID) {
	t.Helper()
	registry := NewCaptureRegistry(h.e.Pool, keyvault.NewMemory(), CaptureConfig{})
	registry.Register(gmail.New(nil, gmail.NewAPI(nil, "")))
	switch how {
	case withdrawals[0]:
		off := false
		if _, err := registry.SetSignatureEnrichment(h.reader(), "gmail", &off); err != nil {
			t.Errorf("switching the mailbox off: %v", err)
		}
	case withdrawals[1]:
		if err := registry.Disconnect(h.reader(), "gmail"); err != nil {
			t.Errorf("disconnecting the mailbox: %v", err)
		}
	default:
		h.e.WsExec(t, `UPDATE activity SET body_completeness = 'partial' WHERE id = $1`, mail)
		return
	}
	if on := h.count(t, `SELECT count(*) FROM capture_connection
		WHERE user_id = $1 AND status = 'connected' AND coalesce(signature_enrich_enabled, true)`, h.owner); on != 0 {
		t.Errorf("%s left the mailbox readable", how)
	}
}

func (h *historyEnv) signatureWrites(t *testing.T) int {
	t.Helper()
	return h.count(t, `SELECT (SELECT count(*) FROM contact_profile_field WHERE contact_id = $1)
		+ (SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1 AND evidence->>'source' = 'capture_enrich')
		+ (SELECT count(*) FROM field_provenance WHERE object_id = $1)`, h.contact)
}

func TestAMailboxWithdrawnDuringTheWalkStopsIt(t *testing.T) {
	for _, how := range withdrawals {
		t.Run(how, func(t *testing.T) {
			h := setupHistory(t)
			oldest := h.writes(t, 0, signed("Head of Procurement"))
			middle := h.writes(t, 1, signed("Buyer"))
			h.writes(t, 2, unsigned)
			h.brain.answers[oldest] = statesTitle("Head of Procurement")
			h.brain.answers[middle] = nil
			h.brain.onAsk = func(mail ids.UUID) {
				if mail == middle {
					h.withdraw(t, how, oldest)
				}
			}

			h.pass(t)

			if got := h.brain.askedAbout(); !sameMails(got, []ids.UUID{middle}) {
				t.Fatalf("the model was asked about %v, want only the mail read before the withdrawal", got)
			}
			if got := h.title(t); got != "" {
				t.Fatalf("title = %q, read from mail withdrawn before its read", got)
			}
		})
	}
}

func TestAMailWithdrawnWhileTheModelReadsItLandsNothing(t *testing.T) {
	for _, how := range withdrawals {
		t.Run(how, func(t *testing.T) {
			h := setupHistory(t)
			older := h.writes(t, 0, signed("Head of Procurement"))
			h.writes(t, 3, unsigned)
			h.brain.answers[older] = statesTitle("Head of Procurement")
			h.brain.onAsk = func(mail ids.UUID) {
				if mail == older {
					h.withdraw(t, how, older)
				}
			}

			h.pass(t)

			if got := h.brain.askedAbout(); !sameMails(got, []ids.UUID{older}) {
				t.Fatalf("the model was asked about %v, want the older mail once", got)
			}
			if got := h.title(t); got != "" {
				t.Fatalf("title = %q, landed from a mail withdrawn while the model read it", got)
			}
			if got := h.signatureWrites(t); got != 0 {
				t.Fatalf("%d evidence, audit or provenance rows from a refused mail", got)
			}
		})
	}
}

func TestATitleAColleagueClearedIsNotRefilledFromOlderMail(t *testing.T) {
	h := setupHistory(t)
	h.entersThenClears(t, h.contact)
	older := h.writes(t, 0, signed("Head of Procurement"))
	h.writes(t, 3, unsigned)
	h.brain.answers[older] = statesTitle("Head of Procurement")

	h.pass(t)

	if got := h.title(t); got != "" {
		t.Fatalf("title = %q, refilled after a colleague cleared it", got)
	}
	if got := h.brain.askedAbout(); len(got) != 0 {
		t.Fatalf("the model was asked about %v for a title a colleague cleared", got)
	}
}

func TestATitleClearedWhileTheModelReadsOlderMailStaysClear(t *testing.T) {
	h := setupHistory(t)
	older := h.writes(t, 0, signed("Head of Procurement"))
	h.writes(t, 3, unsigned)
	h.brain.answers[older] = statesTitle("Head of Procurement")
	h.brain.onAsk = func(mail ids.UUID) {
		if mail == older {
			h.entersThenClears(t, h.contact)
		}
	}

	h.pass(t)

	if got := h.title(t); got != "" {
		t.Fatalf("title = %q, filled over a clear made while the model ran", got)
	}
}

// entersThenClears types a title and then clears it, as a colleague editing the
// contact does.
func (h *historyEnv) entersThenClears(t *testing.T, id ids.UUID) {
	t.Helper()
	contact := ids.From[ids.ContactKind](id)
	if _, err := h.e.Contacts.UpdateContact(h.reader(), contact, contacts.UpdateContactInput{Title: new("Buyer")}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.e.Contacts.UpdateContact(h.reader(), contact, contacts.UpdateContactInput{Clear: []string{"title"}}); err != nil {
		t.Fatal(err)
	}
}

func TestSignatureSourceStillBelongsToTheContactAfterAnEmailEdit(t *testing.T) {
	for _, changed := range []bool{false, true} {
		name := "the sender remains a contact email"
		if changed {
			name = "the sender was removed from the contact"
		}
		t.Run(name, func(t *testing.T) {
			h := setupHistory(t)
			older := h.writes(t, 0, signed("Head of Procurement"))
			h.writes(t, 3, unsigned)
			h.brain.answers[older] = statesTitle("Head of Procurement")
			writesBefore, eventsBefore := -1, -1
			events := func() int {
				return h.count(t, `SELECT count(*) FROM event_outbox
					WHERE envelope->'entity'->>'id' = $1::text`, h.contact)
			}
			h.brain.onAsk = func(mail ids.UUID) {
				if mail != older {
					return
				}
				address := historyFrom
				if changed {
					address = "replacement@customer.example"
				}
				if _, err := h.e.Contacts.UpdateContact(h.reader(), ids.From[ids.ContactKind](h.contact),
					contacts.UpdateContactInput{Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}}}); err != nil {
					t.Fatal(err)
				}
				writesBefore, eventsBefore = h.signatureWrites(t), events()
			}

			h.pass(t)

			if got := h.brain.askedAbout(); !sameMails(got, []ids.UUID{older}) || writesBefore < 0 {
				t.Fatalf("model requests = %v, want the edited older source once", got)
			}
			if changed {
				if got := h.title(t); got != "" {
					t.Fatalf("title = %q from a sender no longer belonging to the contact", got)
				}
				if got := h.signatureWrites(t); got != writesBefore {
					t.Fatalf("signature evidence, audit or provenance rows = %d, want %d", got, writesBefore)
				}
				if got := events(); got != eventsBefore {
					t.Fatalf("outbox rows = %d, want %d after refusing the signature", got, eventsBefore)
				}
				return
			}
			if got := h.title(t); got != "Head of Procurement" {
				t.Fatalf("title = %q, want the still-attributed signature's title", got)
			}
			if h.signatureWrites(t) <= writesBefore || events() <= eventsBefore {
				t.Fatal("the accepted signature must write its evidence, audit and outbox event")
			}
		})
	}
}
