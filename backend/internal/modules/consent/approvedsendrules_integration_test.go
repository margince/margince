// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The approved send rules, each as an allowed and a refused case through the
// real engine.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const twoYears = 2 * 365 * 24 * time.Hour

// seatCtx is a human seat in this env's workspace who may update contacts.
func (e *resolveEnv) seatCtx(user ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// recordRequest writes the rep's "they asked me for this" through the store's
// own door, dated when it happened.
func (e *resolveEnv) recordRequest(t *testing.T, when time.Time) {
	t.Helper()
	if _, err := e.store.RecordQualifyingEvent(e.seatCtx(e.user), e.contact,
		RecordQualifyingEventInput{
			Kind: KindRequestedBySubject, Note: "Asked on the phone for a follow-up next year.", OccurredAt: when,
		}); err != nil {
		t.Fatalf("recording the request: %v", err)
	}
}

// A request on record supports the follow-up it asked for at any age: the rep
// is not asked again because the agreed date is far out.
//
// Mutation: bound requestOnRecord by the reply window again and both cases fail.
func TestARequestOnRecordSupportsAFollowUpAtAnyAge(t *testing.T) {
	t.Run("recorded by the rep", func(t *testing.T) {
		e := setupResolve(t)
		e.recordRequest(t, time.Now().Add(-twoYears))

		if got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup}); !got.Supported {
			t.Fatalf("a request two years old did not support its follow-up: reason %q", got.Reason)
		}
	})
	t.Run("recorded when the contact was acquired", func(t *testing.T) {
		e := setupResolve(t)
		e.acquisition(t, "requested_quote_or_meeting", time.Now().Add(-twoYears))

		if got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup}); !got.Supported {
			t.Fatalf("a request two years old did not support its follow-up: reason %q", got.Reason)
		}
	})
}

// A message the subject wrote two years ago that asked for nothing is not a
// request, so the reply window still bounds an unprompted follow-up on it.
func TestAnOldMessageThatAskedNothingDoesNotSupportAFollowUp(t *testing.T) {
	e := setupResolve(t)
	e.inboundFrom(t, "an-old-thread", e.address, time.Now().Add(-twoYears))

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup})
	if got.Supported || got.Reason != commsauthz.ReasonNoEvidence {
		t.Fatalf("resolved supported=%v reason=%q, want an unsupported follow-up naming the missing evidence",
			got.Supported, got.Reason)
	}
}

// A quote written from the enquiry goes before anyone made a deal or an offer,
// with no newsletter consent, and a marketing objection does not stop it.
//
// Mutation: drop the subjectAskedUs arm from validateQuote and the quote is
// held for review.
func TestAQuoteGoesFromTheEnquiryBeforeAnyDealExists(t *testing.T) {
	e := setupResolve(t)
	e.inboundFrom(t, "the-enquiry", e.address, time.Now().Add(-24*time.Hour))
	e.suppress(t, "marketing_objection")

	d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryPrecontractQuote})
	if d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryPrecontractQuote {
		t.Fatalf("decision = %s/%s %q, want an allowed precontract quote", d.Verdict, d.Resolved, d.ReasonCode)
	}
	if d.Basis != commsauthz.BasisPrecontractRequest {
		t.Errorf("basis = %q, want precontract_request", d.Basis)
	}
}

// A "quote" to somebody who never asked for one is not a requested quote. The
// claim is kept, so the rep is told what is missing rather than told to find a
// newsletter consent.
func TestAQuoteToSomebodyWhoAskedNothingIsNotRequested(t *testing.T) {
	e := setupResolve(t)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryPrecontractQuote})
	if got.Supported {
		t.Fatal("a quote to somebody who asked for nothing was supported")
	}
	if got.Category != commsauthz.CategoryPrecontractQuote || got.Reason != commsauthz.ReasonNoEvidence {
		t.Errorf("resolved %q with %q, want the quote claim kept with the missing evidence named",
			got.Category, got.Reason)
	}
}

// A colleague who takes over the conversation replies on the same thread with
// no consent question. A recipient who never wrote into it does not ride on it.
func TestAColleagueWhoTakesOverRepliesWithoutAConsentQuestion(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "taken-over", e.address, time.Now().Add(-30*24*time.Hour))
	colleague := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`,
		colleague, "colleague-"+colleague.String()+"@r.test"); err != nil {
		t.Fatal(err)
	}
	decideAs := func(req commsauthz.Request) commsauthz.Decision {
		ctx := e.seatCtx(colleague)
		var out commsauthz.Decision
		if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			out, err = e.gate.decideOne(ctx, tx, connector.Recipient{Email: e.address}, req, commsauthz.PhaseStaging)
			return err
		}); err != nil {
			t.Fatalf("deciding as the colleague: %v", err)
		}
		return out
	}

	if d := decideAs(commsauthz.Request{AnchorActivityID: anchor}); d.Verdict != commsauthz.VerdictAllow ||
		d.Resolved != commsauthz.CategoryReplyToInbound {
		t.Fatalf("the colleague's reply = %s/%s %q, want an allowed reply", d.Verdict, d.Resolved, d.ReasonCode)
	}
	if d := decideAs(commsauthz.Request{ThreadKey: "a-thread-they-never-wrote-into"}); d.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("a message on a thread the subject never wrote into was allowed as %s", d.Resolved)
	}
}

// An exception standing over missing marketing consent does not reach a stop
// the subject records after the message was queued: the transmit ticket stops
// offering the refusal to the exception, so the dispatcher parks it.
//
// Mutation: drop set.ExceptionMayAnswer() from AuthorizeTransmit and the second
// ticket still offers the refusal to the exception.
func TestAStopRecordedAfterQueueingIsNotOneAnExceptionAnswers(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	delivery := e.plantDelivery(t)
	recipients := []connector.Recipient{{Email: e.address}}
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		set, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
			Recipients: recipients, LegacyPurposeKey: "newsletter", Subject: "S", Body: "b",
		})
		if err != nil {
			return err
		}
		if d := set.Decisions[0]; d.ReasonCode != commsauthz.ReasonNoMarketingConsent {
			t.Fatalf("staging decision = %+v, want a refusal for missing marketing consent", d)
		}
		return nil
	}); err != nil {
		t.Fatalf("staging the delivery: %v", err)
	}
	transmit := func(attempt int) commsauthz.TransmitTicket {
		ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
			DeliveryID: delivery, Attempt: attempt, Recipients: recipients,
			PurposeKey: "newsletter", Subject: "S", Body: "b",
		})
		if err != nil {
			t.Fatalf("authorizing the transmit: %v", err)
		}
		return ticket
	}

	if ticket := transmit(1); ticket.Allowed || !ticket.ConsentRefused {
		t.Fatalf("before the stop: allowed=%v consentRefused=%v, want a refusal an exception answers",
			ticket.Allowed, ticket.ConsentRefused)
	}
	e.suppress(t, "marketing_objection")
	if ticket := transmit(2); ticket.Allowed || ticket.ConsentRefused {
		t.Fatalf("after the objection: allowed=%v consentRefused=%v, want a refusal no exception answers",
			ticket.Allowed, ticket.ConsentRefused)
	}
}
