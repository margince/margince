// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The signature pass reads older mail for a contact whose newest mail states no
// job title. Every mail here is captured through the production sink from a
// raw message, and the model answers per mail it is shown.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// mailScriptedBrain answers each signature call by the mail it names in the
// fenced source_id, and records which scripted mails it was asked about. A mail
// with no script gets an empty answer: every captured body opens with its From
// line, so the newest mail is always read.
type mailScriptedBrain struct {
	mu      sync.Mutex
	answers map[ids.UUID][]map[string]any
	fail    map[ids.UUID]error
	asked   []ids.UUID
	// onAsk runs before the answer, as a human acting while the model thinks.
	onAsk func(ids.UUID)
}

func newMailScriptedBrain() *mailScriptedBrain {
	return &mailScriptedBrain{answers: map[ids.UUID][]map[string]any{}, fail: map[ids.UUID]error{}}
}

func (b *mailScriptedBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !strings.Contains(req.Messages[0].Content, "Email: "+historyFrom) {
		// A seeded contact's own mail, read in the same pass.
		return model.Response{Text: `{"fields": []}`}, nil
	}
	var mail ids.UUID
	for id := range b.answers {
		if strings.Contains(req.Messages[0].Content, id.String()) {
			mail = id
		}
	}
	for id := range b.fail {
		if strings.Contains(req.Messages[0].Content, id.String()) {
			mail = id
		}
	}
	if mail != ids.Nil {
		b.asked = append(b.asked, mail)
	}
	if b.onAsk != nil {
		b.onAsk(mail)
	}
	if err := b.fail[mail]; err != nil {
		return model.Response{}, err
	}
	payload, err := json.Marshal(map[string]any{"fields": b.answers[mail]})
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: string(payload)}, nil
}

func (b *mailScriptedBrain) askedAbout() []ids.UUID {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]ids.UUID(nil), b.asked...)
}

func statesTitle(title string) []map[string]any {
	return []map[string]any{{"field": "title", "value": title, "evidence_snippet": title, "confidence": 0.9}}
}

// historyEnv is one connected, shared mailbox and one contact who writes to it.
type historyEnv struct {
	*owedEnv
	contact ids.UUID
	brain   *mailScriptedBrain
	day     time.Time
}

const historyFrom = "pat@customer.example"

func setupHistory(t *testing.T) *historyEnv {
	t.Helper()
	o := setupOwed(t)
	h := &historyEnv{owedEnv: o, brain: newMailScriptedBrain(), day: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)}
	h.contact = o.contact(t, "Pat Buyer", historyFrom)
	return h
}

// writes captures an inbound mail from the contact, days after the first one.
func (h *historyEnv) writes(t *testing.T, days int, body string) ids.UUID {
	t.Helper()
	return h.capture(t, owedMail{
		from: historyFrom, to: h.seat, subject: "Order", body: body,
		at: h.day.AddDate(0, 0, days), messageID: "pat-" + ids.NewV7().String() + "@customer.example",
	})
}

func signed(title string) string {
	return "Hello,\n\nsee the order attached.\n\nBest regards,\nPat Buyer\n" + title + "\nCustomer GmbH"
}

const unsigned = "Thanks, that works."

func (h *historyEnv) pass(t *testing.T) {
	t.Helper()
	enricher := NewCaptureEnricher(h.e.Pool, h.brain, slog.New(slog.DiscardHandler))
	if _, err := enricher.RunWorkspace(principal.WithWorkspaceID(context.Background(), h.e.WS)); err != nil {
		t.Fatalf("signature pass: %v", err)
	}
}

func (h *historyEnv) title(t *testing.T) string {
	t.Helper()
	return h.e.WsScalar(t, `SELECT coalesce(title, '') FROM contact WHERE id = $1`, h.contact)
}

func (h *historyEnv) watermark(t *testing.T) string {
	t.Helper()
	return h.e.WsScalar(t, `SELECT coalesce((SELECT activity_id::text || ':' || coalesce(history_depth::text, 'none')
		FROM contact_signature_enrich_state WHERE contact_id = $1), 'none')`, h.contact)
}

func sameMails(got, want []ids.UUID) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestAnOlderSignatureFillsTheTitleWhenTheNewestMailHasNone(t *testing.T) {
	h := setupHistory(t)
	older := h.writes(t, 0, signed("Head of Procurement"))
	newest := h.writes(t, 3, unsigned)
	h.brain.answers[older] = statesTitle("Head of Procurement")

	h.pass(t)

	if got := h.title(t); got != "Head of Procurement" {
		t.Fatalf("title = %q, want the one the older signature states", got)
	}
	if got := h.watermark(t); got != newest.String()+":3" {
		t.Fatalf("watermark = %s, want the newest mail read with the walk", got)
	}
	if got := h.brain.askedAbout(); !sameMails(got, []ids.UUID{older}) {
		t.Fatalf("the model was asked about %v, want only the older signed mail", got)
	}
	h.pass(t)
	if got := len(h.brain.askedAbout()); got != 1 {
		t.Fatalf("a second pass with no new mail asked the model %d times in all, want 1", got)
	}
}

func TestTheThirdMailBackCanSupplyTheTitle(t *testing.T) {
	h := setupHistory(t)
	oldest := h.writes(t, 0, signed("Purchasing Lead"))
	middle := h.writes(t, 1, "Hi,\n\nPat Buyer\n+49 30 1234567")
	h.writes(t, 2, unsigned)
	h.brain.answers[oldest] = statesTitle("Purchasing Lead")
	h.brain.answers[middle] = []map[string]any{{"field": "phone", "value": "+49 30 1234567", "evidence_snippet": "+49 30 1234567", "confidence": 0.9}}

	h.pass(t)

	if got := h.title(t); got != "Purchasing Lead" {
		t.Fatalf("title = %q, want the third mail back's", got)
	}
	if got := h.brain.askedAbout(); !sameMails(got, []ids.UUID{middle, oldest}) {
		t.Fatalf("the model was asked about %v, want the middle mail and then the oldest", got)
	}
	if n := h.count(t, `SELECT count(*) FROM contact_phone WHERE contact_id = $1 AND archived_at IS NULL`, h.contact); n != 0 {
		t.Fatalf("an older mail landed %d phone numbers; it may fill only a blank title", n)
	}
}

func TestANewerSignatureTitleWinsOverAnOlderOne(t *testing.T) {
	h := setupHistory(t)
	older := h.writes(t, 0, signed("Intern"))
	newest := h.writes(t, 4, signed("Managing Director"))
	h.brain.answers[older] = statesTitle("Intern")
	h.brain.answers[newest] = statesTitle("Managing Director")

	h.pass(t)

	if got := h.title(t); got != "Managing Director" {
		t.Fatalf("title = %q, want the newest signature's", got)
	}
	if got := h.brain.askedAbout(); !sameMails(got, []ids.UUID{newest}) {
		t.Fatalf("the model was asked about %v, want only the newest mail", got)
	}
}

func TestAnOlderMailNeverReplacesATitleAHumanEntered(t *testing.T) {
	h := setupHistory(t)
	if _, err := h.e.Contacts.UpdateContact(h.reader(), ids.From[ids.ContactKind](h.contact),
		contacts.UpdateContactInput{Title: new("Head of Sales")}); err != nil {
		t.Fatal(err)
	}
	older := h.writes(t, 0, signed("Intern"))
	h.writes(t, 2, unsigned)
	h.brain.answers[older] = statesTitle("Intern")

	h.pass(t)

	if got := h.title(t); got != "Head of Sales" {
		t.Fatalf("title = %q, want the one a human entered", got)
	}
	if got := h.brain.askedAbout(); len(got) != 0 {
		t.Fatalf("the model was asked about %v for a contact who already has a title", got)
	}
}

func TestATitleEnteredWhileTheModelReadsOlderMailStands(t *testing.T) {
	h := setupHistory(t)
	older := h.writes(t, 0, signed("Intern"))
	h.writes(t, 2, unsigned)
	h.brain.answers[older] = statesTitle("Intern")
	h.brain.onAsk = func(mail ids.UUID) {
		if mail != older {
			return
		}
		if _, err := h.e.Contacts.UpdateContact(h.reader(), ids.From[ids.ContactKind](h.contact),
			contacts.UpdateContactInput{Title: new("Head of Sales")}); err != nil {
			t.Error(err)
		}
	}

	h.pass(t)

	if got := h.title(t); got != "Head of Sales" {
		t.Fatalf("title = %q, want the one a human entered while the model ran", got)
	}
}

func TestTheWalkStopsAtTheDepth(t *testing.T) {
	h := setupHistory(t)
	oldest := h.writes(t, 0, signed("Purchasing Lead"))
	second := h.writes(t, 1, unsigned)
	third := h.writes(t, 2, "Regards,\nPat Buyer")
	h.writes(t, 3, unsigned)
	h.brain.answers[oldest] = statesTitle("Purchasing Lead")
	h.brain.answers[third] = nil
	h.brain.answers[second] = nil

	h.pass(t)

	if got := h.title(t); got != "" {
		t.Fatalf("title = %q, but the only titled mail is the fourth one back", got)
	}
	for _, mail := range h.brain.askedAbout() {
		if mail == oldest {
			t.Fatal("the pass read a mail past its history depth")
		}
	}
}

func TestAModelErrorInTheWalkLeavesTheContactACandidate(t *testing.T) {
	h := setupHistory(t)
	older := h.writes(t, 0, signed("Head of Procurement"))
	h.writes(t, 3, unsigned)
	h.brain.fail[older] = errors.New("model unavailable")

	h.pass(t)

	if got := h.watermark(t); got != "none" {
		t.Fatalf("watermark = %s after a failed read, want none", got)
	}
	delete(h.brain.fail, older)
	h.brain.answers[older] = statesTitle("Head of Procurement")
	h.pass(t)
	if got := h.title(t); got != "Head of Procurement" {
		t.Fatalf("title = %q after the retry, want the older signature's", got)
	}
}

// secondMailbox connects Rep2's shared mailbox and returns its address.
func (h *historyEnv) secondMailbox(t *testing.T) string {
	t.Helper()
	seat := seatAddress(t, h.e, h.e.Rep2)
	h.e.WsExec(t, `UPDATE capture_connection SET mail_posture = 'shared' WHERE user_id = $1`, h.e.Rep2)
	return seat
}

// writesTo captures an inbound mail from the contact into Rep2's mailbox.
func (h *historyEnv) writesTo(t *testing.T, seat string, days int, body string) ids.UUID {
	t.Helper()
	return h.captureFor(t, h.e.Rep2, seat, owedMail{
		from: historyFrom, to: seat, subject: "Order", body: body,
		at: h.day.AddDate(0, 0, days), messageID: "pat-" + ids.NewV7().String() + "@customer.example",
	})
}

func TestASwitchedOffMailboxNoLongerHidesAnOlderMail(t *testing.T) {
	h := setupHistory(t)
	seat := h.secondMailbox(t)
	older := h.writes(t, 0, signed("Head of Procurement"))
	switchedOff := h.writesTo(t, seat, 3, signed("Intern"))
	h.e.WsExec(t, `UPDATE capture_connection SET signature_enrich_enabled = false WHERE user_id = $1`, h.e.Rep2)
	h.brain.answers[older] = statesTitle("Head of Procurement")
	h.brain.answers[switchedOff] = statesTitle("Intern")

	h.pass(t)

	if got := h.title(t); got != "Head of Procurement" {
		t.Fatalf("title = %q, want the older mail's from the mailbox that is on", got)
	}
	if got := h.brain.askedAbout(); !sameMails(got, []ids.UUID{older}) {
		t.Fatalf("the model was asked about %v, want only the mail from the mailbox that is on", got)
	}
}

// TestTheWalkReadsOnlyMailThisPassMayRead gives the contact a newest mail with
// no signature and one older signed mail the pass must not read.
func TestTheWalkReadsOnlyMailThisPassMayRead(t *testing.T) {
	for name, older := range map[string]func(*testing.T, *historyEnv) ids.UUID{
		"a body not read in full": func(t *testing.T, h *historyEnv) ids.UUID {
			mail := h.writes(t, 0, signed("Head of Procurement"))
			h.e.WsExec(t, `UPDATE activity SET body_completeness = 'partial' WHERE id = $1`, mail)
			return mail
		},
		"newsletter mail": func(t *testing.T, h *historyEnv) ids.UUID {
			raw := owedMail{
				from: historyFrom, to: h.seat, subject: "News", body: signed("Head of Procurement"),
				at: h.day, messageID: "news-" + ids.NewV7().String() + "@customer.example",
			}.raw()
			return h.captureRaw(t, append([]byte("List-Unsubscribe: <mailto:leave@customer.example>\r\n"), raw...))
		},
		"mail a colleague of theirs sent": func(t *testing.T, h *historyEnv) ids.UUID {
			h.owedEnv.contact(t, "Sam Other", "sam@customer.example")
			return h.capture(t, owedMail{
				from: "sam@customer.example", to: h.seat, cc: historyFrom, subject: "Order",
				body: "Hi Pat Buyer,\n\nthe order is attached.\n\nSam Other\nHead of Procurement\nCustomer GmbH", at: h.day,
				messageID: "sam-" + ids.NewV7().String() + "@customer.example",
			})
		},
		"archived mail": func(t *testing.T, h *historyEnv) ids.UUID {
			mail := h.writes(t, 0, signed("Head of Procurement"))
			h.e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, mail)
			return mail
		},
		"mail from a switched-off mailbox": func(t *testing.T, h *historyEnv) ids.UUID {
			seat := h.secondMailbox(t)
			mail := h.writesTo(t, seat, 0, signed("Head of Procurement"))
			h.e.WsExec(t, `UPDATE capture_connection SET signature_enrich_enabled = false WHERE user_id = $1`, h.e.Rep2)
			return mail
		},
		"mail from a disconnected mailbox": func(t *testing.T, h *historyEnv) ids.UUID {
			seat := h.secondMailbox(t)
			mail := h.writesTo(t, seat, 0, signed("Head of Procurement"))
			h.e.WsExec(t, `UPDATE capture_connection SET status = 'disconnected' WHERE user_id = $1`, h.e.Rep2)
			return mail
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := setupHistory(t)
			mail := older(t, h)
			if linked := h.count(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id = $2`, mail, h.contact); linked != 1 {
				t.Fatalf("the older mail is not on the contact (%d links), so the case proves nothing", linked)
			}
			h.writes(t, 3, unsigned)
			h.brain.answers[mail] = statesTitle("Head of Procurement")

			h.pass(t)

			if got := h.brain.askedAbout(); len(got) != 0 {
				t.Fatalf("the model was asked about %v", got)
			}
			if got := h.title(t); got != "" {
				t.Fatalf("title = %q, read from mail the pass may not read", got)
			}
		})
	}
}

func TestTwoMailsAtTheSameTimeAreBothRead(t *testing.T) {
	h := setupHistory(t)
	signedMail := h.writes(t, 0, signed("Head of Procurement"))
	newest := h.writes(t, 0, unsigned)
	h.brain.answers[signedMail] = statesTitle("Head of Procurement")

	h.pass(t)

	if got := h.watermark(t); got != newest.String()+":3" {
		t.Fatalf("watermark = %s, want the mail with the larger id read as the newest", got)
	}

	if got := h.title(t); got != "Head of Procurement" {
		t.Fatalf("title = %q, want the signed mail's, sent at the same second as the newest", got)
	}
}

func TestTheWalkStopsAtTheFirstTitle(t *testing.T) {
	h := setupHistory(t)
	oldest := h.writes(t, 0, signed("Intern"))
	middle := h.writes(t, 1, signed("Head of Procurement"))
	h.writes(t, 2, unsigned)
	h.brain.answers[oldest] = statesTitle("Intern")
	h.brain.answers[middle] = statesTitle("Head of Procurement")

	h.pass(t)

	if got := h.title(t); got != "Head of Procurement" {
		t.Fatalf("title = %q, want the newest title found", got)
	}
	if got := h.brain.askedAbout(); !sameMails(got, []ids.UUID{middle}) {
		t.Fatalf("the model was asked about %v, want the walk to stop at the first title", got)
	}
}
