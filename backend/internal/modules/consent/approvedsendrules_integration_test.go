// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The approved send rules, each as an allowed and a refused case through the
// real engine.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

// A request on record supports the follow-up it asked for at any age. The rep
// is not asked again because the agreed date is far out.
//
// Mutation: drop the requestOnRecord arm from subjectAskedUs and both fail.
func TestARequestOnRecordSupportsAFollowUpAtAnyAge(t *testing.T) {
	t.Run("recorded by the rep", func(t *testing.T) {
		e := setupResolve(t)
		e.recordRequest(t, clocktest.Now(t).Add(-twoYears))

		if got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup}); !got.Supported {
			t.Fatalf("a request two years old did not support its follow-up: reason %q", got.Reason)
		}
	})
	t.Run("recorded when the contact was acquired", func(t *testing.T) {
		e := setupResolve(t)
		e.acquisition(t, "requested_quote_or_meeting", clocktest.Now(t).Add(-twoYears))

		if got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup}); !got.Supported {
			t.Fatalf("a request two years old did not support its follow-up: reason %q", got.Reason)
		}
	})
}

// A message the subject wrote two years ago that asked for nothing is not a
// request. The reply window still bounds an unprompted follow-up on it.
func TestAnOldMessageThatAskedNothingDoesNotSupportAFollowUp(t *testing.T) {
	e := setupResolve(t)
	e.inboundFrom(t, "an-old-thread", e.address, clocktest.Now(t).Add(-twoYears))

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup})
	if got.Supported || got.Reason != commsauthz.ReasonNoEvidence {
		t.Fatalf("resolved supported=%v reason=%q, want an unsupported follow-up naming the missing evidence",
			got.Supported, got.Reason)
	}
}

// A quote the contact asked for goes before anyone made a deal or an offer. It
// needs no newsletter consent. The request is the rep's record, or the one
// recorded when the contact was acquired.
//
// Mutation: drop the requestOnRecord arm from validateQuote and both cases are
// held for review.
func TestAQuoteGoesFromTheRequestBeforeAnyDealExists(t *testing.T) {
	for name, record := range map[string]func(e *resolveEnv){
		"recorded by the rep":   func(e *resolveEnv) { e.recordRequest(t, clocktest.Now(t).Add(-24*time.Hour)) },
		"recorded at acquiring": func(e *resolveEnv) { e.acquisition(t, "requested_quote_or_meeting", clocktest.Now(t)) },
	} {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			record(e)

			d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryPrecontractQuote})
			if d.Verdict != commsauthz.VerdictAllow || d.Resolved != commsauthz.CategoryPrecontractQuote {
				t.Fatalf("decision = %s/%s %q, want an allowed precontract quote", d.Verdict, d.Resolved, d.ReasonCode)
			}
			if d.Basis != commsauthz.BasisPrecontractRequest {
				t.Errorf("basis = %q, want precontract_request", d.Basis)
			}
		})
	}
}

// A "quote" to somebody who asked for none is not a requested quote, and
// correspondence alone is not that request. The claim is kept, so the rep is
// told what is missing rather than told to find a newsletter consent.
//
// Mutation: let validateQuote ask subjectAskedUs instead of requestOnRecord and
// the correspondence case is allowed.
func TestAQuoteToSomebodyWhoAskedForNoneIsNotRequested(t *testing.T) {
	for name, seed := range map[string]func(e *resolveEnv){
		"nothing on record":       func(*resolveEnv) {},
		"only correspondence":     func(e *resolveEnv) { e.inboundFrom(t, "a-chat", e.address, clocktest.Now(t).Add(-24*time.Hour)) },
		"permission, not request": func(e *resolveEnv) { e.acquisition(t, "in_person_permission", clocktest.Now(t)) },
	} {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			seed(e)

			got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryPrecontractQuote})
			if got.Supported {
				t.Fatal("a quote nobody asked for was supported")
			}
			if got.Category != commsauthz.CategoryPrecontractQuote || got.Reason != commsauthz.ReasonNoEvidence {
				t.Errorf("resolved %q with %q, want the quote claim kept with the missing evidence named",
					got.Category, got.Reason)
			}
		})
	}
}

// A quote resting on a request alone, with no deal or offer behind it, is still
// stopped by a broad marketing objection. A narrow one names a marketing list
// the quote is not on.
//
// Mutation: drop objectionBindsResolution from decideResolved and the quote is
// allowed past the objection.
func TestAMarketingObjectionBindsAQuoteWithNoDealBehindIt(t *testing.T) {
	e := setupResolve(t)
	e.recordRequest(t, clocktest.Now(t).Add(-24*time.Hour))
	e.suppress(t, "marketing_objection")

	d := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryPrecontractQuote})
	if d.Verdict != commsauthz.VerdictDeny || d.ReasonCode != commsauthz.ReasonObjection {
		t.Fatalf("decision = %s %q, want a refusal for the marketing objection", d.Verdict, d.ReasonCode)
	}
	if d.Resolved != commsauthz.CategoryPrecontractQuote {
		t.Errorf("resolved %q, want the quote it is", d.Resolved)
	}
}

// A request the rep retracts stops counting, and the withdrawal is audited.
// Retracting it twice finds nothing live.
//
// Mutation: drop "retracted_at IS NULL" from requestOnRecord and the follow-up
// is still supported.
func TestARetractedRequestNoLongerCounts(t *testing.T) {
	e := setupResolve(t)
	recorded, err := e.store.RecordQualifyingEvent(e.seatCtx(e.user), e.contact, RecordQualifyingEventInput{
		Kind: KindRequestedBySubject, Note: "Asked for a call back in spring.", OccurredAt: clocktest.Now(t).Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("recording the request: %v", err)
	}
	if got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup}); !got.Supported {
		t.Fatalf("the recorded request did not support its follow-up: %q", got.Reason)
	}
	event, err := ids.Parse(recorded.ID)
	if err != nil {
		t.Fatalf("the recorded request carries no id: %v", err)
	}

	if err := e.store.RetractQualifyingEvent(e.seatCtx(e.user), e.contact, event); err != nil {
		t.Fatalf("retracting the request: %v", err)
	}
	if got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup}); got.Supported {
		t.Fatal("a retracted request still supported the follow-up")
	}
	var audited bool
	if err := e.owner.QueryRow(context.Background(), `
		SELECT EXISTS (SELECT 1 FROM audit_log
		                WHERE entity_type = 'contact' AND entity_id = $1
		                  AND after ? 'qualifying_event_retracted')`, e.contact).Scan(&audited); err != nil {
		t.Fatalf("reading the audit trail: %v", err)
	}
	if !audited {
		t.Error("the withdrawal left no audit row")
	}
	if err := e.store.RetractQualifyingEvent(e.seatCtx(e.user), e.contact, event); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a second retraction: err = %v, want not found", err)
	}
}

// A colleague who takes over the conversation replies on the same thread with
// no consent question. A recipient who never wrote into it does not ride on it.
func TestAColleagueWhoTakesOverRepliesWithoutAConsentQuestion(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "taken-over", e.address, clocktest.Now(t).Add(-30*24*time.Hour))
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

// stagedMarketingDelivery stages a newsletter send nobody consented to, and
// answers the delivery and a way to ask the transmit phase about it.
func (e *resolveEnv) stagedMarketingDelivery(t *testing.T) (ids.UUID, func(attempt int) commsauthz.TransmitTicket) {
	t.Helper()
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
	return delivery, func(attempt int) commsauthz.TransmitTicket {
		ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
			DeliveryID: delivery, Attempt: attempt, Recipients: recipients,
			PurposeKey: "newsletter", Subject: "S", Body: "b",
		})
		if err != nil {
			t.Fatalf("authorizing the transmit: %v", err)
		}
		return ticket
	}
}

// putUnderException records a one-message exception on the delivery, given by
// an admin or by a rep, the way staging leaves a directed delivery.
func (e *resolveEnv) putUnderException(t *testing.T, delivery ids.UUID, givenByAdmin bool) {
	t.Helper()
	ctx := context.Background()
	var review, instruction ids.UUID
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO communication_review (state, kind, reason_code, refusals)
		VALUES ('needs_repair', 'single', 'no_marketing_consent',
		        jsonb_build_array(jsonb_build_object('address', $1::text, 'reason_code', 'no_marketing_consent')))
		RETURNING id`, e.address).Scan(&review); err != nil {
		t.Fatalf("seeding the review: %v", err)
	}
	if err := e.owner.QueryRow(ctx, `
		INSERT INTO communication_instruction
		  (review_id, directed_by, reason_code, explanation, warning_version,
		   acknowledged_at, facts_as_of, valid_until, given_by_admin)
		VALUES ($1, $2, 'other', 'Agreed on the call.', 'override-v1', now(), now(), now() + interval '1 day', $3)
		RETURNING id`, review, e.user, givenByAdmin).Scan(&instruction); err != nil {
		t.Fatalf("seeding the exception: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		UPDATE comms_outbound SET execution_authority = 'instruction', instruction_id = $2 WHERE id = $1`,
		delivery, instruction); err != nil {
		t.Fatalf("putting the delivery under the exception: %v", err)
	}
}

// An exception over missing marketing consent does not reach a stop recorded
// after the message was queued. The transmit ticket stops offering the refusal
// to the exception, so the dispatcher parks it.
//
// Mutation: drop set.ExceptionMayAnswer from AuthorizeTransmit and the second
// ticket still offers the refusal to the exception.
func TestAStopRecordedAfterQueueingIsNotOneAnExceptionAnswers(t *testing.T) {
	e := setupResolve(t)
	delivery, transmit := e.stagedMarketingDelivery(t)
	e.putUnderException(t, delivery, true)

	if ticket := transmit(1); ticket.Allowed || !ticket.ConsentRefused {
		t.Fatalf("before the stop: allowed=%v consentRefused=%v, want a refusal the exception answers",
			ticket.Allowed, ticket.ConsentRefused)
	}
	e.suppress(t, "marketing_objection")
	if ticket := transmit(2); ticket.Allowed || ticket.ConsentRefused {
		t.Fatalf("after the objection: allowed=%v consentRefused=%v, want a refusal no exception answers",
			ticket.Allowed, ticket.ConsentRefused)
	}
}

// A refusal only an admin may answer, recorded after the exception was given,
// is answered by an admin's exception and not by a rep's. The authority is the
// one stored on the exception, never the one of whoever sends now.
//
// Mutation: pass true to ExceptionMayAnswer in AuthorizeTransmit, or drop
// given_by_admin from exceptionGivenByAdminTx's answer, and one case fails.
func TestAnAdminOnlyRefusalIsAnsweredOnlyByAnAdminsException(t *testing.T) {
	for _, tc := range []struct {
		name         string
		givenByAdmin bool
	}{{"a rep's exception", false}, {"an admin's exception", true}} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupResolve(t)
			delivery, transmit := e.stagedMarketingDelivery(t)
			e.putUnderException(t, delivery, tc.givenByAdmin)
			e.suppress(t, "hard_bounce")

			ticket := transmit(1)
			if ticket.Allowed {
				t.Fatal("a bounced address was allowed")
			}
			if ticket.ConsentRefused != tc.givenByAdmin {
				t.Errorf("consentRefused = %v, want %v: only an admin's exception answers a bounce",
					ticket.ConsentRefused, tc.givenByAdmin)
			}
		})
	}
}
