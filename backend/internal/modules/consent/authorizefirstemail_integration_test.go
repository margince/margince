// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// A first business email to a contact known only from captured mail: what lets
// it go, and what keeps it out. The Cc rule is scoped to one thread. A recorded
// fact is read by the composer path. A picked deal supports a follow-up. A
// sender's unsupported claim is a question rather than a dead end.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// copiedMail describes one captured mail that copies the subject: who wrote it,
// which way it went, and whether one of our seats received it.
type copiedMail struct {
	threadKey   string
	subjectRole string
	direction   string
	// ourAuthor puts one of our seats in From instead of an outside sender.
	ourAuthor bool
	// ourRecipient puts one of our seats in To, as capture does for the
	// mailbox that received the mail.
	ourRecipient bool
	capturedBy   string
	// counterparty is the correspondent the row is filed under; it defaults
	// to the outside sender every helper here uses.
	counterparty string
	// unauthenticated leaves inbound_sender_authenticated false.
	unauthenticated bool
}

// capturedMail plants a mail a connector captured. An outside sender writes to
// one of our seats with the subject in To or Cc.
func (e *resolveEnv) capturedMail(t *testing.T, threadKey, subjectRole string, withOurSide bool) ids.UUID {
	t.Helper()
	id := e.plantCopiedMail(t, copiedMail{
		threadKey: threadKey, subjectRole: subjectRole, direction: "inbound",
		ourRecipient: withOurSide, capturedBy: "connector:gmail",
	})
	if withOurSide {
		e.plantCopiedMail(t, copiedMail{
			threadKey: threadKey, subjectRole: "to", direction: "outbound",
			ourAuthor: true, capturedBy: "connector:gmail",
		})
	}
	return id
}

func TestAForgedThreadKeyWithNoMailFromOurSeatEarnsNoCopiedStanding(t *testing.T) {
	e := setupResolve(t)
	e.plantCopiedMail(t, copiedMail{
		threadKey: "thread-forged", subjectRole: "cc", direction: "inbound",
		ourRecipient: true, capturedBy: "connector:gmail",
	})

	d := e.decide(t, commsauthz.Request{ThreadKey: "thread-forged"})

	if d.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("decision = %+v, want no allow: our seat only received mail on this thread", d)
	}
}

// ourSentMail plants a captured mail our seat sent on the thread. The subject
// is only in Bcc, so it is never the copying mail.
func (e *resolveEnv) ourSentMail(t *testing.T, threadKey string) {
	t.Helper()
	e.plantCopiedMail(t, copiedMail{
		threadKey: threadKey, subjectRole: "bcc", direction: "outbound",
		ourAuthor: true, capturedBy: "connector:gmail",
	})
}

// Our side counts only as the sender of an outbound mail. A seat in To of an
// outbound mail did not send it, nor did a seat in From of an inbound one.
func TestOurSeatCountsOnlyAsTheSenderOfAnOutboundMail(t *testing.T) {
	cases := map[string]copiedMail{
		"our seat only in To of an outbound mail": {direction: "outbound", ourRecipient: true},
		"our seat in From of an inbound mail":     {direction: "inbound", ourAuthor: true},
	}
	for name, ours := range cases {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			e.capturedMail(t, "thread-shape", "cc", false)
			ours.threadKey, ours.subjectRole, ours.capturedBy = "thread-shape", "bcc", "connector:gmail"
			e.plantCopiedMail(t, ours)

			if got := e.resolve(t, commsauthz.Request{ThreadKey: "thread-shape"}); got.Supported {
				t.Fatalf("a mail our seat did not send counted as our side: %+v", got)
			}
		})
	}
}

func (e *resolveEnv) plantCopiedMail(t *testing.T, m copiedMail) ids.UUID {
	t.Helper()
	ctx := context.Background()
	id := ids.NewV7()
	counterparty := m.counterparty
	if counterparty == "" {
		counterparty = "third@party.test"
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, thread_key, occurred_at, source, captured_by,
		                      counterparty_email, counterparty_outbound_attested, inbound_sender_authenticated)
		VALUES ($1, 'email', $2, $3, now() - interval '1 hour', 'gmail', $4, $5, $2 = 'outbound', $2 = 'inbound' AND NOT $6)`,
		id, m.direction, m.threadKey, m.capturedBy, counterparty, m.unauthenticated); err != nil {
		t.Fatalf("planting the captured mail: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity_participant (activity_id, contact_id, address, role)
		VALUES ($1, $2, $3, $4)`, id, e.contact, e.address, m.subjectRole); err != nil {
		t.Fatalf("planting the subject participant: %v", err)
	}
	author := `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'from')`
	authorArgs := []any{id, counterparty}
	if m.ourAuthor {
		author = `INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'from')`
		authorArgs = []any{id, e.user}
	}
	if _, err := e.owner.Exec(ctx, author, authorArgs...); err != nil {
		t.Fatalf("planting the author: %v", err)
	}
	if m.ourRecipient {
		if _, err := e.owner.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'to')`,
			id, e.user); err != nil {
			t.Fatalf("planting our recipient: %v", err)
		}
	}
	return id
}

func TestACcParticipantOfACapturedThreadMayBeWrittenToAboutThatThread(t *testing.T) {
	for _, role := range []string{"to", "cc"} {
		t.Run(role, func(t *testing.T) {
			e := setupResolve(t)
			e.capturedMail(t, "thread-matter", role, true)

			d := e.decide(t, commsauthz.Request{ThreadKey: "thread-matter"})

			if d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryReplyToInbound {
				t.Fatalf("decision = %+v, want an allow as reply_to_inbound", d)
			}
			if d.Basis != commsauthz.BasisLegitimateInterests {
				t.Errorf("basis = %q, want legitimate_interests", d.Basis)
			}
		})
	}
}

func TestAnOutsiderForgingAThreadWeWroteOnToSomeoneElseEarnsNoStanding(t *testing.T) {
	cases := map[string]struct{ inbound, ours copiedMail }{
		"the outsider is not who we wrote to": {
			inbound: copiedMail{counterparty: "outsider@evil.test"},
			ours:    copiedMail{counterparty: "third@party.test"},
		},
		"the sender is not authenticated": {
			inbound: copiedMail{unauthenticated: true},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			c.inbound.threadKey, c.inbound.subjectRole, c.inbound.direction = "thread-we-wrote", "cc", "inbound"
			c.inbound.ourRecipient, c.inbound.capturedBy = true, "connector:gmail"
			e.plantCopiedMail(t, c.inbound)
			c.ours.threadKey, c.ours.subjectRole, c.ours.direction = "thread-we-wrote", "bcc", "outbound"
			c.ours.ourAuthor, c.ours.capturedBy = true, "connector:gmail"
			e.plantCopiedMail(t, c.ours)

			d := e.decide(t, commsauthz.Request{ThreadKey: "thread-we-wrote"})

			if d.Verdict == commsauthz.VerdictAllow {
				t.Fatalf("decision = %+v, want no allow for a forged thread key", d)
			}
		})
	}
}

func TestACcParticipantStillNeedsAnAnswerForAnUnrelatedTopic(t *testing.T) {
	e := setupResolve(t)
	e.capturedMail(t, "thread-matter", "cc", true)

	got := e.resolve(t, commsauthz.Request{ThreadKey: "another-thread"})

	if got.Supported {
		t.Fatal("being copied on one thread supported mail about another topic")
	}
}

// Our own side must never be able to make the evidence by copying the contact.
func TestOurOwnMailCopyingTheContactIsNotEvidence(t *testing.T) {
	cases := map[string]copiedMail{
		"outbound mail we sent": {direction: "outbound", ourAuthor: true},
		// Capture files a colleague's copy as outbound, with the colleague
		// named only by address.
		"a colleague's mail filed as outbound": {direction: "outbound", ourRecipient: true},
		"inbound mail one of our seats wrote": {
			direction: "inbound", ourAuthor: true, ourRecipient: true,
		},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			m.threadKey, m.subjectRole, m.capturedBy = "thread-ours", "cc", "connector:gmail"
			e.plantCopiedMail(t, m)
			e.ourSentMail(t, "thread-ours")

			if got := e.resolve(t, commsauthz.Request{ThreadKey: "thread-ours"}); got.Supported {
				t.Fatalf("our own mail copying the contact supported a first mail: %+v", got)
			}
		})
	}
}

func TestTheCcRuleNeedsOurSideOnTheThreadAndAConnectorCapture(t *testing.T) {
	e := setupResolve(t)
	e.capturedMail(t, "thread-no-us", "cc", false)
	if got := e.resolve(t, commsauthz.Request{ThreadKey: "thread-no-us"}); got.Supported {
		t.Error("a thread none of our seats took part in supported a first mail")
	}

	e.plantCopiedMail(t, copiedMail{
		threadKey: "thread-typed", subjectRole: "cc", direction: "inbound", capturedBy: "human:x",
	})
	e.ourSentMail(t, "thread-typed")
	if got := e.resolve(t, commsauthz.Request{ThreadKey: "thread-typed"}); got.Supported {
		t.Error("a hand-typed activity manufactured the evidence a connector capture provides")
	}
}

// A thread key is shared across media, so our side on a hand-typed activity or
// on another medium with the same key does not count.
func TestOurSideMustBeOnACapturedMailOfTheSameThread(t *testing.T) {
	cases := map[string]string{
		"hand-typed":    `INSERT INTO activity (id, kind, direction, thread_key, occurred_at, source, captured_by) VALUES ($1, 'email', 'outbound', 'thread-split', now(), 'gmail', 'human:x')`,
		"another media": `INSERT INTO activity (id, kind, direction, thread_key, occurred_at, source, captured_by) VALUES ($1, 'call', 'outbound', 'thread-split', now(), 'gmail', 'connector:gmail')`,
	}
	for name, ours := range cases {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			e.capturedMail(t, "thread-split", "cc", false)
			oursID := ids.NewV7()
			ctx := context.Background()
			if _, err := e.owner.Exec(ctx, ours, oursID); err != nil {
				t.Fatalf("planting our activity: %v", err)
			}
			if _, err := e.owner.Exec(ctx, `
				INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'from')`,
				oursID, e.user); err != nil {
				t.Fatalf("planting our participant: %v", err)
			}

			if got := e.resolve(t, commsauthz.Request{ThreadKey: "thread-split"}); got.Supported {
				t.Fatalf("our side off the captured mail thread supported a first mail: %+v", got)
			}
		})
	}
}

func TestAnArchivedCapturedMailSupportsNothing(t *testing.T) {
	e := setupResolve(t)
	id := e.capturedMail(t, "thread-gone", "cc", true)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE activity SET archived_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}

	if got := e.resolve(t, commsauthz.Request{ThreadKey: "thread-gone"}); got.Supported {
		t.Error("an archived mail supported a send")
	}
}

func TestTheCcRuleNeverSupportsAdvertising(t *testing.T) {
	e := setupResolve(t)
	e.capturedMail(t, "thread-matter", "cc", true)

	d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryMarketing, ThreadKey: "thread-matter"})

	if d.Verdict != commsauthz.VerdictDeny || d.ReasonCode != commsauthz.ReasonClaimContradictsResolution {
		t.Fatalf("decision = %+v, want a marketing claim denied as claim_contradicts_resolution", d)
	}
}

func TestARecordedRequestSupportsAComposerFollowUp(t *testing.T) {
	e := setupResolve(t)
	claim := commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup}
	if got := e.resolve(t, claim); got.Supported {
		t.Fatal("a follow-up with no evidence was supported")
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO consent_qualifying_event (contact_id, kind, occurred_at, note, source, captured_by)
		VALUES ($1, 'requested_by_subject', now() - interval '2 days', 'asked on the phone', 'human', 'human:x')`,
		e.contact); err != nil {
		t.Fatalf("recording the fact: %v", err)
	}

	if got := e.resolve(t, claim); !got.Supported {
		t.Fatalf("a recorded request did not support the follow-up: %+v", got)
	}
}

func TestAPickedOpenDealSupportsADealFollowUp(t *testing.T) {
	cases := map[string]struct {
		status      string
		stakeholder bool
		want        bool
	}{
		"open and a stakeholder": {"open", true, true},
		"closed":                 {"won", true, false},
		"not a stakeholder":      {"open", false, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			deal := e.openDeal(t, c.status, c.stakeholder)

			got := e.resolve(t, commsauthz.Request{
				Context:  commsauthz.CategoryActiveDealFollowup,
				Evidence: commsauthz.Evidence{DealID: deal},
			})

			if got.Supported != c.want {
				t.Errorf("supported = %v, want %v", got.Supported, c.want)
			}
		})
	}
}

func TestAClaimNamingNoDealIsNotSupportedAsADealFollowUp(t *testing.T) {
	e := setupResolve(t)
	if got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryActiveDealFollowup}); got.Supported {
		t.Fatal("a deal follow-up naming no deal was supported")
	}
}

func TestTheStrongerEvidenceWinsOverAMistakenClaim(t *testing.T) {
	e := setupResolve(t)
	deal := e.openDeal(t, "open", true)

	d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup, Links: []ids.UUID{deal}})

	if d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryActiveDealFollowup {
		t.Fatalf("decision = %+v, want an allow as active_deal_followup", d)
	}
	if d.Requested != commsauthz.CategoryRequestedFollowup {
		t.Errorf("requested = %q, want the claim kept on the row", d.Requested)
	}
}

func TestAnUnsupportedClaimWithNoPurposeIsAQuestionNotAnUnknownPurpose(t *testing.T) {
	for _, claim := range []commsauthz.Category{commsauthz.CategoryCustomerService, commsauthz.CategoryAccountNotice} {
		t.Run(string(claim), func(t *testing.T) {
			e := setupResolve(t)

			d := e.decide(t, commsauthz.Request{Context: claim})

			if d.Verdict != commsauthz.VerdictReview || d.ReasonCode != commsauthz.ReasonNoEvidence {
				t.Fatalf("decision = %+v, want a review as no_compatible_evidence", d)
			}
		})
	}
}

func TestAPurposeNothingDefinesIsStillUnknown(t *testing.T) {
	e := setupResolve(t)

	d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryCustomerService, LegacyPurposeKey: "invented"})

	if d.ReasonCode != commsauthz.ReasonUnknownPurpose {
		t.Fatalf("reason = %q, want unknown_purpose for an invented purpose key", d.ReasonCode)
	}
}

// A send the Cc rule allowed at staging is asked again at transmit, where the
// request carries no anchor and only the delivery's thread key. It goes while
// the copying mail stands and is refused once that mail is gone.
func TestASendTheCcRuleAllowedIsAskedAgainAtTransmit(t *testing.T) {
	e := setupResolve(t)
	copying := e.capturedMail(t, "thread-transmit", "cc", true)
	delivery := e.plantDelivery(t)
	if _, err := e.owner.Exec(context.Background(), `
		UPDATE activity SET thread_key = 'thread-transmit'
		 WHERE id = (SELECT activity_id FROM comms_outbound WHERE id = $1)`, delivery); err != nil {
		t.Fatalf("filing the delivery on the thread: %v", err)
	}
	recipients := []connector.Recipient{{Email: e.address}}
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		set, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
			Recipients: recipients, ThreadKey: "thread-transmit", Subject: "Re: s", Body: "b",
		})
		if err != nil {
			return err
		}
		if d := set.Decisions[0]; d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryReplyToInbound {
			t.Fatalf("staging decision = %+v, want an allow as reply_to_inbound", d)
		}
		return nil
	}); err != nil {
		t.Fatalf("staging the delivery: %v", err)
	}
	transmit := func(attempt int) commsauthz.TransmitTicket {
		ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
			DeliveryID: delivery, Attempt: attempt, Recipients: recipients, Subject: "Re: s", Body: "b",
		})
		if err != nil {
			t.Fatalf("authorizing the transmit: %v", err)
		}
		return ticket
	}

	if ticket := transmit(1); !ticket.Allowed {
		t.Fatalf("a send the Cc rule allowed at staging was refused at transmit: %q", ticket.Reason)
	}
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE activity SET archived_at = now() WHERE id = $1`, copying); err != nil {
		t.Fatalf("archiving the copying mail: %v", err)
	}
	if ticket := transmit(2); ticket.Allowed {
		t.Fatal("a send went out after the evidence for the Cc rule was gone")
	}
}
