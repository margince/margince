// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	rescheduleReply = "Können wir den Termin auf Donnerstag, 14 Uhr verschieben?"
	offerReply      = rescheduleReply + "\n\nAnbei unser Angebot mit 20 % Rabatt."
)

// offerEnv is a contact who wrote into a thread, and the anchor a rep answers.
func offerEnv(t *testing.T) (*resolveEnv, ids.UUID) {
	t.Helper()
	e := setupResolve(t)
	e.livesIn(t, reviewableMarketingCountry())
	e.seedPurpose(t, "newsletter", "marketing")
	return e, e.inboundFrom(t, "their-question", e.address, clocktest.Now(t).Add(-time.Hour))
}

func replyTo(anchor ids.UUID, body string) commsauthz.Request {
	return commsauthz.Request{AnchorActivityID: anchor, AuthoredSubject: "Re: Termin", AuthoredBody: body}
}

// One sender, anchor and recipient; only the authored words and the record
// differ. An offer written into a reply is judged as what it is.
func TestAnOfferWrittenIntoAReplyIsJudgedAsAnOffer(t *testing.T) {
	t.Parallel()
	t.Run("a reschedule is a reply", func(t *testing.T) {
		e, anchor := offerEnv(t)
		d := e.decide(t, replyTo(anchor, rescheduleReply))
		if d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryReplyToInbound {
			t.Fatalf("decision = %s/%s %q, want an allowed reply", d.Verdict, d.Resolved, d.ReasonCode)
		}
	})
	t.Run("an offer without consent is refused", func(t *testing.T) {
		e, anchor := offerEnv(t)
		d := e.decide(t, replyTo(anchor, offerReply))
		if d.Verdict == commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryMarketing ||
			d.ReasonCode != commsauthz.ReasonNoMarketingConsent {
			t.Fatalf("decision = %s/%s %q, want marketing refused for want of consent", d.Verdict, d.Resolved, d.ReasonCode)
		}
		if !(commsauthz.DecisionSet{Decisions: []commsauthz.Decision{d}}).ExceptionMayAnswer(false) {
			t.Error("a rep cannot send this one message with a recorded reason")
		}
	})
	t.Run("a correspondence purpose does not carry an offer", func(t *testing.T) {
		e, anchor := offerEnv(t)
		e.seedPurpose(t, "business_correspondence", "business_correspondence")
		req := replyTo(anchor, offerReply)
		req.LegacyPurposeKey = "business_correspondence"
		if d := e.decide(t, req); d.Verdict == commsauthz.VerdictAllow {
			t.Fatalf("an offer went out as correspondence: %s %q", d.Resolved, d.ReasonCode)
		}
	})
	t.Run("an offer the contact asked for is a quote", func(t *testing.T) {
		e, anchor := offerEnv(t)
		e.recordRequest(t, clocktest.Now(t).Add(-48*time.Hour), true)
		d := e.decide(t, replyTo(anchor, offerReply))
		if d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryPrecontractQuote {
			t.Fatalf("decision = %s/%s %q, want an allowed quote", d.Verdict, d.Resolved, d.ReasonCode)
		}
	})
	t.Run("an offer with marketing consent is marketing", func(t *testing.T) {
		e, anchor := offerEnv(t)
		e.grantNewsletter(t)
		req := replyTo(anchor, offerReply)
		req.MarketingPurpose = "newsletter"
		d := e.decide(t, req)
		if d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryMarketing {
			t.Fatalf("decision = %s/%s %q, want allowed marketing", d.Verdict, d.Resolved, d.ReasonCode)
		}
	})
}

// Transmit has no authored text. It judges the words staging judged, so a
// reply stays a reply and an offer stays refused.
func TestTransmitJudgesTheWordsStagingJudged(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		body     string
		allowed  bool
		resolved commsauthz.Category
	}{
		"a reschedule": {rescheduleReply, true, commsauthz.CategoryReplyToInbound},
		"an offer":     {offerReply, false, commsauthz.CategoryMarketing},
	} {
		t.Run(name, func(t *testing.T) {
			e, anchor := offerEnv(t)
			delivery := e.deliveryOnThread(t, "their-question")
			recipients := []connector.Recipient{{Email: e.address}}
			req := replyTo(anchor, c.body)
			req.Recipients, req.Subject, req.Body = recipients, "Re: Termin", c.body
			if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
				_, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, req)
				return err
			}); err != nil {
				t.Fatalf("staging: %v", err)
			}
			ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
				DeliveryID: delivery, Attempt: 1, Recipients: recipients, Subject: "Re: Termin", Body: c.body,
			})
			if err != nil {
				t.Fatalf("transmit: %v", err)
			}
			var resolved string
			if err := e.owner.QueryRow(context.Background(), `
				SELECT resolved_category FROM communication_decision
				 WHERE delivery_id = $1 AND phase = 'transmit'`, delivery).Scan(&resolved); err != nil {
				t.Fatalf("reading the transmit decision: %v", err)
			}
			if ticket.Allowed != c.allowed || commsauthz.Category(resolved) != c.resolved {
				t.Fatalf("transmit = allowed %v as %s, want %v as %s", ticket.Allowed, resolved, c.allowed, c.resolved)
			}
		})
	}
}

// grantNewsletter records the contact's newsletter consent through the store.
func (e *resolveEnv) grantNewsletter(t *testing.T) {
	t.Helper()
	if _, err := e.store.Record(e.seatCtx(e.user), RecordInput{
		ContactID: e.contact, PurposeID: ids.From[ids.PurposeKind](e.purposeID(t, "newsletter")),
		NewState: "granted", PolicyText: &grantWording,
	}); err != nil {
		t.Fatalf("recording the consent: %v", err)
	}
}

// deliveryOnThread plants a delivery whose activity belongs to a thread, which
// is how transmit names the conversation.
func (e *resolveEnv) deliveryOnThread(t *testing.T, threadKey string) ids.UUID {
	t.Helper()
	delivery := e.plantDelivery(t)
	if _, err := e.owner.Exec(context.Background(), `
		UPDATE activity SET thread_key = $2
		 WHERE id = (SELECT activity_id FROM comms_outbound WHERE id = $1)`, delivery, threadKey); err != nil {
		t.Fatalf("threading the delivery: %v", err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE comms_outbound SET consent_purpose = '' WHERE id = $1`, delivery); err != nil {
		t.Fatalf("clearing the delivery's purpose: %v", err)
	}
	return delivery
}

// A subject quoted at staging is asked about again at transmit. While the
// contact's own mail stands, the reply goes as a reply. Archived in between,
// the subject is the rep's offer and transmit refuses it.
func TestTransmitAsksAgainWhoWroteAQuotedSubject(t *testing.T) {
	t.Parallel()
	for name, archive := range map[string]bool{"the anchor stands": false, "the anchor was archived": true} {
		t.Run(name, func(t *testing.T) {
			e, anchor := offerEnv(t)
			if _, err := e.owner.Exec(context.Background(), `
				UPDATE activity SET subject = 'Termin und Angebot', captured_by = 'connector:gmail', inbound_sender_authenticated = true
				 WHERE id = $1`, anchor); err != nil {
				t.Fatalf("making the anchor the contact's authenticated mail: %v", err)
			}
			// An earlier mail of theirs keeps the thread a reply after the
			// anchor is archived, so only the subject can change the answer.
			e.inboundFrom(t, "their-question", e.address, clocktest.Now(t).Add(-2*time.Hour))
			delivery := e.deliveryOnThread(t, "their-question")
			recipients := []connector.Recipient{{Email: e.address}}
			req := commsauthz.Request{
				AnchorActivityID: anchor, AuthoredSubject: "Re: Termin und Angebot", AuthoredBody: rescheduleReply,
				Recipients: recipients, Subject: "Re: Termin und Angebot", Body: rescheduleReply,
			}
			var staged commsauthz.DecisionSet
			if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
				var err error
				staged, err = e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, req)
				return err
			}); err != nil {
				t.Fatalf("staging: %v", err)
			}
			if d := staged.Decisions[0]; d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryReplyToInbound {
				t.Fatalf("staging = %s/%s %q, want an allowed reply: the fixture proves nothing", d.Verdict, d.Resolved, d.ReasonCode)
			}
			if archive {
				if _, err := e.owner.Exec(context.Background(), `UPDATE activity SET archived_at = now() WHERE id = $1`, anchor); err != nil {
					t.Fatal(err)
				}
			}
			ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
				DeliveryID: delivery, Attempt: 1, Recipients: recipients, Subject: req.Subject, Body: req.Body,
			})
			if err != nil {
				t.Fatalf("transmit: %v", err)
			}
			var resolved string
			if err := e.owner.QueryRow(context.Background(), `
				SELECT resolved_category FROM communication_decision
				 WHERE delivery_id = $1 AND phase = 'transmit'`, delivery).Scan(&resolved); err != nil {
				t.Fatalf("reading the transmit decision: %v", err)
			}
			if ticket.Allowed == archive || (commsauthz.Category(resolved) == commsauthz.CategoryMarketing) != archive {
				t.Fatalf("transmit = allowed %v as %s with the anchor archived: %v", ticket.Allowed, resolved, archive)
			}
		})
	}
}
