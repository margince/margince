// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The six ways an evidence check could refuse ordinary correspondence, and the
// answers that stop it.
//
// A wrong refusal is the failure mode that makes reps distrust the product and
// route around it, and it costs more than the permission it was protecting. So
// each case here asserts an ALLOW, or a review whose reason a human can act on
// — never a deny. A future tightening that turns one of these into a refusal
// fails a test rather than a rep's day.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SCENARIO 1: a reply to an inbound older than the window.
//
// The window bounds an UNPROMPTED follow-up, never a reply. A same-thread reply
// has no age limit at all — the subject wrote to us and never withdrew — so a
// rep answering a two-year-old thread is doing the ordinary thing.
func TestAReplyToAVeryOldThreadIsStillAReply(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "thread-old", e.address, time.Now().Add(-3*365*24*time.Hour))

	got := e.resolve(t, commsauthz.Request{AnchorActivityID: anchor})

	if !got.Supported || got.Category != commsauthz.CategoryReplyToInbound {
		t.Fatalf("resolved %q supported=%v — a same-thread reply has no age limit", got.Category, got.Supported)
	}
}

// SCENARIO 2: the inbound arrived on a different thread.
//
// Thread continuity is one way to bind evidence, not the only one. A rep
// answering yesterday's mail in a fresh compose window is the normal case, so
// an unprompted follow-up falls back to "this contact wrote to us inside the
// window" without needing the same thread.
func TestAnUnpromptedFollowUpRestsOnAnyRecentInbound(t *testing.T) {
	e := setupResolve(t)
	e.inboundFrom(t, "some-other-thread", e.address, time.Now().Add(-24*time.Hour))

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup})

	if !got.Supported {
		t.Fatalf("an inbound a day ago did not support a follow-up: reason %q", got.Reason)
	}
	if got.Basis != commsauthz.BasisSubjectInitiatedCorrespondence {
		t.Errorf("basis = %q, want subject_initiated_correspondence", got.Basis)
	}
}

// SCENARIO 3: the first mail to somebody who phoned.
//
// PR 4's acquisition evidence IS the request. A rep who logged "they asked me
// for a quote at the trade fair" has recorded it, and asking them to record it
// again in a different table would be asking them to restate what the CRM
// already knows.
func TestAContactWhoAskedInPersonCanBeWrittenTo(t *testing.T) {
	for _, kind := range []string{"requested_quote_or_meeting", "in_person_permission"} {
		t.Run(kind, func(t *testing.T) {
			e := setupResolve(t)
			e.acquisition(t, kind, time.Now().Add(-48*time.Hour))

			got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup})

			if !got.Supported {
				t.Fatalf("%s did not support a first message: reason %q", kind, got.Reason)
			}
		})
	}
}

// AND THE CONVERSE, or the test above passes with every acquisition kind
// treated as permission. Where a contact CAME FROM is provenance; a purchased
// list and a public source say nothing about what this contact asked for.
func TestProvenanceIsNotPermission(t *testing.T) {
	for _, kind := range []string{"purchased_or_imported", "public_or_business_source", "referral"} {
		t.Run(kind, func(t *testing.T) {
			e := setupResolve(t)
			e.acquisition(t, kind, time.Now().Add(-48*time.Hour))

			got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup})

			if got.Supported {
				t.Fatalf("%s supported writing to somebody who never asked", kind)
			}
		})
	}
}

// SCENARIO 5: an invoice recipient the record does not name.
//
// Missing relational data is a CRM gap, not a legal refusal. The answer is a
// review naming what to link, and the send still goes while the engine is
// observed — never a deny that leaves finance unable to send an invoice.
func TestAnInvoiceRecipientTheRecordDoesNotNameIsReviewedNotRefused(t *testing.T) {
	e := setupResolve(t)
	invoice := e.invoice(t, e.company(t), false)

	got := e.resolve(t, commsauthz.Request{
		Context:  commsauthz.CategoryInvoiceOrPayment,
		Evidence: commsauthz.Evidence{InvoiceID: invoice},
	})

	if got.Supported {
		t.Fatal("an invoice reached a contact with no link to the customer")
	}
	if got.Reason != commsauthz.ReasonNoEvidence {
		t.Errorf("reason = %q, want a reason naming the missing evidence", got.Reason)
	}
	// The claim survives under its own name, so the operator is told to link
	// the contact to the customer rather than to find a marketing consent.
	if got.Category != commsauthz.CategoryInvoiceOrPayment {
		t.Errorf("category = %q, want the invoice claim kept for the reader", got.Category)
	}
}

// And the same invoice does support the send once the contact is the
// customer's billing contact. Without this the test above would pass against a
// validator that never allows.
func TestAnInvoiceReachesTheCustomersBillingContact(t *testing.T) {
	e := setupResolve(t)
	company := e.company(t)
	invoice := e.invoice(t, company, false)
	e.billTo(t, company)

	got := e.resolve(t, commsauthz.Request{
		Context:  commsauthz.CategoryInvoiceOrPayment,
		Evidence: commsauthz.Evidence{InvoiceID: invoice},
	})

	if !got.Supported {
		t.Fatalf("the customer's billing contact could not be sent their invoice: %q", got.Reason)
	}
	if got.Basis != commsauthz.BasisContract {
		t.Errorf("basis = %q, want contract", got.Basis)
	}
}

// AND IT STILL REACHES THEM ONE PHASE LATER.
//
// The evidence that supported this message at staging is not written to the
// decision row — communication_decision has an evidence column and nothing
// fills it — so stagedRequestFor rebuilds the transmit question with the
// category and the thread key and NOTHING ELSE. validateInvoice then reads a
// zero InvoiceID, finds no invoice, and the message falls through to the
// legacy purpose arm.
//
// That fall-through used to end in the transactional class's unconditional
// allow, which is what hid the evidence loss: the invoice went out on the
// purpose key rather than on the invoice, and the two answers happened to
// agree. Closing the key means they no longer do, and a real invoice would
// park at transmit after staging clean.
//
// So this test is the one that fails if the fix ships without the evidence
// being carried. It is the rep-facing half of the change: refusing a message
// nobody can evidence is the point, refusing an invoice somebody DID evidence
// is the regression.
func TestAnEvidencedInvoiceSurvivesToTransmit(t *testing.T) {
	e := setupResolve(t)
	company := e.company(t)
	invoice := e.invoice(t, company, false)
	e.billTo(t, company)
	e.seedPurpose(t, "transactional", "transactional")

	req := commsauthz.Request{
		Context:  commsauthz.CategoryInvoiceOrPayment,
		Evidence: commsauthz.Evidence{InvoiceID: invoice},
		// The shape a caller who has not migrated still sends: the modern
		// context AND the old key. The key must not be what carries it, and
		// its presence must not take the evidence away either.
		LegacyPurposeKey: "transactional",
	}

	// Through the REAL staging writer, not a hand-planted row. A fixture that
	// wrote its own communication_decision would be asserting about the
	// fixture: the whole question is whether AuthorizeStagingTx records the
	// evidence, so the test that answers it has to be the one that calls it.
	delivery := e.plantDelivery(t)
	req.Recipients = []connector.Recipient{{Email: e.address}}
	var staged commsauthz.DecisionSet
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		staged, err = e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, req)
		return err
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}
	if len(staged.Decisions) != 1 || staged.Decisions[0].Verdict != commsauthz.VerdictAllow {
		t.Fatalf("staging refused an evidenced invoice: %+v", staged.Decisions)
	}

	// Now the transmit question, built the way authorizetransmit.go builds it.
	var got commsauthz.Decision
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		claims, err := stagedClaims(e.ctx, tx, delivery)
		if err != nil {
			return err
		}
		r := connector.Recipient{Email: e.address}
		got, err = e.gate.decideOne(e.ctx, tx, r,
			stagedRequestFor(commsauthz.TransmitRequest{
				DeliveryID: delivery, PurposeKey: "transactional",
			}, r, claims, "", nil), commsauthz.PhaseTransmit)
		return err
	}); err != nil {
		t.Fatalf("deciding at transmit: %v", err)
	}

	if got.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("an invoice authorized at staging was refused at transmit (%s / %s): the evidence "+
			"that supported it is not carried past staging, and the transactional key is no longer "+
			"there to accidentally cover for that", got.Verdict, got.ReasonCode)
	}
}

// EVIDENCE NOBODY CHECKED THE SENDER MAY SEE IS NOT WRITTEN DOWN.
//
// This is the security half of carrying evidence, and it is why the decision
// row takes its ids from the DECISION rather than from the request.
//
// refuseUnreadableEvidence lives inside validate, and validate runs only for
// arm 3 of resolution — a message already answered by the thread arm or the
// live-deal arm never reaches it. So a sender can be allowed by an open deal
// while naming an invoice id in the same request, and nothing has asked whether
// they hold finance.read.
//
// If that id reached the row, the transmit phase would read it back and put it
// to the invoice validator — under the SYSTEM principal, for which auth.Require
// returns nil unconditionally. An id the sender could not open at staging would
// authorize their message one phase later. That is a privilege escalation with
// no race and no tampering: file a message under a deal you can see, name an
// invoice you cannot, and let the worker do the reading.
//
// The fixture is exactly that shape: a live deal the recipient is a stakeholder
// on, so the deal arm answers first. A real invoice of a company they are the
// billing contact of, so the invoice arm would allow if it ever ran. And a
// sender principal holding neither finance nor contract.
func TestEvidenceTheSenderCannotReadIsNotCarriedPastStaging(t *testing.T) {
	e := setupResolve(t)
	company := e.company(t)
	invoice := e.invoice(t, company, false)
	e.billTo(t, company)
	deal := e.openDeal(t, "open", true)
	e.enquiredAbout(t, deal)

	// A seat that may write mail and read contacts, and may NOT read finance.
	// Everything else about the context is the ordinary sending principal.
	e.ctx = principal.WithActor(e.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"contact": {Read: true},
				"deal":    {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})

	delivery := e.plantDelivery(t)
	req := commsauthz.Request{
		Recipients: []connector.Recipient{{Email: e.address}},
		// No claimed context: the deal arm answers before any claim is looked
		// at, which is the whole point — the sender never has to name a
		// category the validators would check.
		Links:    []ids.UUID{deal},
		Evidence: commsauthz.Evidence{InvoiceID: invoice},
	}
	var staged commsauthz.DecisionSet
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		staged, err = e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, req)
		return err
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}
	if len(staged.Decisions) != 1 || staged.Decisions[0].Verdict != commsauthz.VerdictAllow {
		t.Fatalf("the live deal did not allow the message, so this fixture is not testing what it "+
			"claims to: %+v", staged.Decisions)
	}
	if got := staged.Decisions[0].Resolved; got != commsauthz.CategoryActiveDealFollowup {
		t.Fatalf("resolved %q, want active_deal_followup — the deal arm has to be the one that "+
			"answered, or this fixture proves nothing", got)
	}

	// THE ROW MUST NOT HOLD THE INVOICE.
	var raw []byte
	if err := e.owner.QueryRow(context.Background(), `
		SELECT evidence FROM communication_decision
		 WHERE delivery_id = $1 AND phase = 'staging'`, delivery).Scan(&raw); err != nil {
		t.Fatalf("reading the decision row: %v", err)
	}
	if carried := evidenceFrom(raw); carried.InvoiceID != (ids.UUID{}) {
		t.Fatalf("the decision row carries invoice %v, which nobody checked this sender may read: "+
			"the transmit phase runs as the system principal and would allow on it", carried.InvoiceID)
	}
}

// EMPLOYMENT ALONE CHOOSES NOBODY. A current job at the customer does not make
// somebody the one its invoices and contracts go to.
//
// Mutation: put the employment arm back in documentRecipientSQL and this fails.
func TestEmploymentAloneChoosesNoDocumentRecipient(t *testing.T) {
	e := setupResolve(t)
	company := e.company(t)
	invoice := e.invoice(t, company, false)
	contract := e.contract(t, company, nil)
	e.employ(t, company)

	for _, req := range []commsauthz.Request{
		{Context: commsauthz.CategoryInvoiceOrPayment, Evidence: commsauthz.Evidence{InvoiceID: invoice}},
		{Context: commsauthz.CategoryContractNotice, Evidence: commsauthz.Evidence{ContractID: contract}},
	} {
		if got := e.resolve(t, req); got.Supported {
			t.Errorf("%s reached an employee who is not the billing contact", req.Context)
		}
	}
}

// A REMOVED BILLING CONTACT IS NOT REACHED.
//
// Mutation: drop b.archived_at IS NULL from documentRecipientSQL and this fails.
func TestARemovedBillingContactIsNotReachedByTheInvoices(t *testing.T) {
	e := setupResolve(t)
	company := e.company(t)
	invoice := e.invoice(t, company, false)
	e.billTo(t, company)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE relationship SET archived_at = now() WHERE contact_id = $1`, e.contact); err != nil {
		t.Fatal(err)
	}

	got := e.resolve(t, commsauthz.Request{
		Context:  commsauthz.CategoryInvoiceOrPayment,
		Evidence: commsauthz.Evidence{InvoiceID: invoice},
	})

	if got.Supported {
		t.Fatal("a removed billing contact was still reached by the customer's invoices")
	}
}

// A company with no billing contact sends to the buyer on the deal. The
// contract's own deal answers; an invoice names its deal. A colleague who only
// advised on the deal is not the buyer, and a billing contact on the company
// takes the mail from the buyer.
//
// Mutation: drop the NOT EXISTS clause from documentRecipientSQL and the last
// case fails; drop the role filter and the adviser case fails.
func TestACompanyWithNoBillingContactSendsToTheBuyerOnTheDeal(t *testing.T) {
	for _, tc := range []struct {
		name          string
		role          string
		billingOnFile bool
		want          bool
	}{
		{"the economic buyer", "economic_buyer", false, true},
		{"an adviser on the deal", "influencer", false, false},
		{"the buyer, with a billing contact on file", "economic_buyer", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupResolve(t)
			company := e.company(t)
			deal := e.buyerOnDeal(t, company, tc.role)
			if tc.billingOnFile {
				other := e.anotherContact(t, "billing@buyer.test")
				other.billTo(t, company)
			}
			invoice := e.invoice(t, company, false)
			contract := e.contract(t, company, &deal)

			for _, req := range []commsauthz.Request{
				{Context: commsauthz.CategoryInvoiceOrPayment, Evidence: commsauthz.Evidence{InvoiceID: invoice, DealID: deal}},
				{Context: commsauthz.CategoryContractNotice, Evidence: commsauthz.Evidence{ContractID: contract}},
			} {
				if got := e.resolve(t, req); got.Supported != tc.want {
					t.Errorf("%s: supported = %v, want %v (%s)", req.Context, got.Supported, tc.want, got.Reason)
				}
			}
		})
	}
}

// Service, account and security mail reach a customer with no typed fact. A
// customer is a contact working at a company with an active contract or a won
// deal. Somebody serving notice still works there, because ended_at is a date.
//
// Mutation: spell the employment test as `r.ended_at IS NULL` and the notice
// case fails; drop the won-deal arm and the won-deal case fails.
func TestAContactAtACustomerIsSentServiceMailWithNoFact(t *testing.T) {
	for _, tc := range []struct {
		name     string
		customer func(t *testing.T, e *resolveEnv, company ids.UUID)
		want     bool
	}{
		{"an active contract", func(t *testing.T, e *resolveEnv, company ids.UUID) {
			e.contract(t, company, nil)
		}, true},
		{"a won deal", func(t *testing.T, e *resolveEnv, company ids.UUID) {
			e.buyerOnDeal(t, company, "user")
			if _, err := e.owner.Exec(context.Background(),
				`UPDATE deal SET status = 'won', closed_at = now() WHERE company_id = $1`, company); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"serving notice at a customer", func(t *testing.T, e *resolveEnv, company ids.UUID) {
			e.contract(t, company, nil)
			if _, err := e.owner.Exec(context.Background(),
				`UPDATE relationship SET ended_at = current_date + 30 WHERE contact_id = $1 AND kind = 'employment'`,
				e.contact); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"only an open deal", func(t *testing.T, e *resolveEnv, company ids.UUID) {
			e.buyerOnDeal(t, company, "user")
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupResolve(t)
			company := e.company(t)
			e.employ(t, company)
			tc.customer(t, e, company)

			for _, category := range []commsauthz.Category{
				commsauthz.CategoryCustomerService, commsauthz.CategoryAccountNotice, commsauthz.CategorySecurityNotice,
			} {
				if got := e.resolve(t, commsauthz.Request{Context: category}); got.Supported != tc.want {
					t.Errorf("%s: supported = %v, want %v", category, got.Supported, tc.want)
				}
			}
		})
	}
}

// A CLAIM NAMING SOMEBODY ELSE'S INVOICE SUPPORTS NOTHING. The evidence id is
// caller-supplied, so naming an invoice belonging to a company this
// contact has nothing to do with must not admit the message.
func TestAnInvoiceForAnotherCustomerSupportsNothing(t *testing.T) {
	e := setupResolve(t)
	e.billTo(t, e.company(t))
	elsewhere := e.invoice(t, e.company(t), false)

	got := e.resolve(t, commsauthz.Request{
		Context:  commsauthz.CategoryInvoiceOrPayment,
		Evidence: commsauthz.Evidence{InvoiceID: elsewhere},
	})

	if got.Supported {
		t.Fatal("an invoice belonging to a different customer admitted the message")
	}
}

// A category claimed with NO record named supports nothing. Naming the kind of
// message is not evidence that it is one.
func TestAClaimNamingNoRecordSupportsNothing(t *testing.T) {
	e := setupResolve(t)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryContractNotice})

	if got.Supported {
		t.Fatal("a contract-notice claim with no contract named supported the message")
	}
}

// A category the vocabulary does not contain is not treated as an ordinary
// unevidenced claim: the engine knows nothing about it and says so.
func TestACategoryOutsideTheVocabularyIsNotAnOrdinaryClaim(t *testing.T) {
	e := setupResolve(t)

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.Category("not_a_category")})

	if got.Supported {
		t.Fatal("an unreadable category supported the message")
	}
	if got.Reason != commsauthz.ReasonUnknownPurpose {
		t.Errorf("reason = %q, want unknown_purpose", got.Reason)
	}
}

// THE GROUND GOES ON THE RECORD. communication_basis had an erasure writer, a
// retention writer and a subject-access reader — and no writer at all, so the
// export answered "we relied on nothing" for every message ever sent.
//
// Mutation: drop the recordBasis call from decideResolved and this fails.
func TestASupportedSendWritesDownTheGroundItRelliedOn(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "thread-1", e.address, time.Now().Add(-time.Hour))

	if d := e.decide(t, commsauthz.Request{AnchorActivityID: anchor}); d.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("the reply was not allowed: %q", d.ReasonCode)
	}

	var kind, threadKey string
	var validUntil *time.Time
	if err := e.owner.QueryRow(context.Background(), `
		SELECT kind, coalesce(thread_key, ''), valid_until
		  FROM communication_basis WHERE contact_id = $1`, e.contact).Scan(&kind, &threadKey, &validUntil); err != nil {
		t.Fatalf("reading the recorded ground: %v", err)
	}
	if kind != string(commsauthz.BasisSubjectInitiatedCorrespondence) {
		t.Errorf("kind = %q, want subject_initiated_correspondence", kind)
	}
	if threadKey != "thread-1" {
		t.Errorf("thread_key = %q, want the conversation it was earned on", threadKey)
	}
	// BOUNDED. The qualifying-event row this sits beside never expires, which
	// is the shape that turns one reply into an open licence.
	if validUntil == nil {
		t.Fatal("the recorded ground never expires, which is the shape this table exists to avoid")
	}
}

// ONE GROUND, NOT ONE PER MESSAGE. Two replies on one thread rest on the same
// ground, and a row per message would make the export a second copy of the
// mailbox.
func TestTheSameGroundIsRecordedOnce(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "thread-1", e.address, time.Now().Add(-time.Hour))
	req := commsauthz.Request{AnchorActivityID: anchor}

	e.decide(t, req)
	e.decide(t, req)

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_basis WHERE contact_id = $1`, e.contact).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("two sends on one thread wrote %d basis rows, want 1", rows)
	}
}

// A SECOND CONVERSATION IS A SECOND GROUND. Scoping by thread is what stops the
// first reply becoming a licence for every later subject.
//
// Mutation: drop the thread_key comparison from basisAlreadyLive and this fails.
func TestASecondThreadEarnsItsOwnGround(t *testing.T) {
	e := setupResolve(t)
	first := e.inboundFrom(t, "thread-1", e.address, time.Now().Add(-time.Hour))
	second := e.inboundFrom(t, "thread-2", e.address, time.Now().Add(-time.Hour))

	e.decide(t, commsauthz.Request{AnchorActivityID: first})
	e.decide(t, commsauthz.Request{AnchorActivityID: second})

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_basis WHERE contact_id = $1`, e.contact).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("two conversations wrote %d basis rows, want one each", rows)
	}
}

// An UNSUPPORTED resolution records nothing. The message falls through to the
// legacy verdict, whose own grounds are already recorded as consent rows, and a
// basis row here would claim a ground the engine never reached.
func TestAnUnsupportedResolutionRecordsNoGround(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")

	e.decide(t, commsauthz.Request{LegacyPurposeKey: "newsletter"})

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_basis WHERE contact_id = $1`, e.contact).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("an unsupported send recorded %d grounds, want none", rows)
	}
}

// company plants a customer record.
func (e *resolveEnv) company(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Acme', 'manual', 'human:x')`, id); err != nil {
		t.Fatalf("planting the company: %v", err)
	}
	return id
}

// employ links this env's contact to a company, the way a finance contact
// reaches the customer whose invoices they receive.
func (e *resolveEnv) employ(t *testing.T, company ids.UUID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO relationship (kind, company_id, contact_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:x')`, company, e.contact); err != nil {
		t.Fatalf("planting the employment: %v", err)
	}
}

// billTo makes this env's contact a billing contact of a company, the one its
// invoices and contracts go to.
func (e *resolveEnv) billTo(t *testing.T, company ids.UUID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO relationship (kind, company_id, contact_id, role, source, captured_by)
		VALUES ('billing_contact', $1, $2, 'recipient', 'manual', 'human:x')`, company, e.contact); err != nil {
		t.Fatalf("planting the billing contact: %v", err)
	}
}

// buyerOnDeal plants an open deal of the company with this env's contact on
// it in the given role, and answers the deal.
func (e *resolveEnv) buyerOnDeal(t *testing.T, company ids.UUID, role string) ids.UUID {
	t.Helper()
	ctx := context.Background()
	deal := e.openDeal(t, "open", false)
	if _, err := e.owner.Exec(ctx, `UPDATE deal SET company_id = $2 WHERE id = $1`, deal, company); err != nil {
		t.Fatalf("filing the deal under the company: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO relationship (kind, deal_id, contact_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, $3, 'manual', 'human:x')`, deal, e.contact, role); err != nil {
		t.Fatalf("seating the contact on the deal: %v", err)
	}
	return deal
}

// contract plants an active contract of the company, filed under a deal when
// one is named, and answers its id.
func (e *resolveEnv) contract(t *testing.T, company ids.UUID, deal *ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contract (id, company_id, deal_id, title, status, source, captured_by)
		VALUES ($1, $2, $3, 'Master agreement', 'active', 'manual', 'human:x')`, id, company, deal); err != nil {
		t.Fatalf("planting the contract: %v", err)
	}
	return id
}

// invoice plants a finance invoice against a company, with the finance
// connection it hangs off.
func (e *resolveEnv) invoice(t *testing.T, company ids.UUID, _ bool) ids.UUID {
	t.Helper()
	ctx := context.Background()
	conn := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO finance_connection (id, provider, credential_ref, source, captured_by)
		VALUES ($1, 'lexoffice', 'ref', 'manual', 'human:x')`, conn); err != nil {
		t.Fatalf("planting the finance connection: %v", err)
	}
	id := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO finance_invoice
		  (id, connection_id, company_id, external_id, issued_at, status,
		   currency, net_minor, gross_minor, sync_hash, source, captured_by)
		VALUES ($1, $2, $3, $4, current_date, 'open', 'EUR', 1000, 1190, 'hash', 'lexoffice', 'human:x')`,
		id, conn, company, "ext-"+id.String()); err != nil {
		t.Fatalf("planting the invoice: %v", err)
	}
	return id
}

// acquisition records why this env's contact exists, which is what PR 4's
// evidence table holds and what a "they asked me in person" answer rests on.
func (e *resolveEnv) acquisition(t *testing.T, kind string, when time.Time) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
		VALUES ($1, $2, $3, 'human:x')`, e.contact, kind, when); err != nil {
		t.Fatalf("planting the acquisition evidence: %v", err)
	}
}

// A FILED ACTIVITY IS NOT SOMETHING THE CONTACT WROTE.
//
// activity_link is a FILING link with no author concept, and a caller may post
// an activity with direction=inbound and a link to any contact they can read
// (CreateActivityRequest passes direction, occurred_at and links through). So a
// follow-up arm that asked activity_link would let anybody manufacture their own
// evidence for writing to anybody.
//
// Mutation: ask activity_link instead of activity_participant with role 'from'
// — the shape this shipped as — and this passes with forged evidence.
func TestAFiledActivityIsNotSomethingTheContactWrote(t *testing.T) {
	e := setupResolve(t)
	ctx := context.Background()
	// An inbound activity FILED under the contact, which they did not write:
	// no participant row names them as the author.
	id := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, source, occurred_at, captured_by)
		VALUES ($1, 'note', 'inbound', 'manual', now(), 'human:x')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, contact_id)
		VALUES ($1, 'contact', $2)`, id, e.contact); err != nil {
		t.Fatal(err)
	}

	got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup})

	if got.Supported {
		t.Fatal("an activity merely filed under somebody supported writing to them")
	}
}

// AN EVIDENCE ID THE CALLER MAY NOT READ IS REFUSED, and refused as NOT FOUND
// so naming a guessed id discloses nothing about whether it exists.
//
// Evidence ids arrive on the request body and nothing upstream probes them —
// unlike Links, which SendOrigin.resolve puts through auth.EnsureLinkTarget.
// Without this a seat with no finance grant could name any invoice in the
// installation and have the engine answer about it.
//
// Mutation: drop the refuseUnreadableEvidence call and this passes.
func TestAnEvidenceRecordTheCallerMayNotReadIsRefused(t *testing.T) {
	e := setupResolve(t)
	company := e.company(t)
	invoice := e.invoice(t, company, false)
	e.billTo(t, company)
	e.dropGrant(t, "finance")

	var err error
	if txErr := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err = e.gate.resolveCategory(e.ctx, tx, commsauthz.Request{
			Context:  commsauthz.CategoryInvoiceOrPayment,
			Evidence: commsauthz.Evidence{InvoiceID: invoice},
		}, subjectRef{Kind: entityContact, ID: e.contact.String(), Address: e.address})
		return nil
	}); txErr != nil {
		t.Fatalf("running the resolution: %v", txErr)
	}
	if err == nil {
		t.Fatal("a seat with no finance grant had the engine read their invoice")
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("refused with %v, want a not-found so existence stays hidden", err)
	}
}

// A voided invoice authorizes nothing, and neither does an archived one. The
// offer cases are in quoteondeal_integration_test.go.
//
// Mutation: drop any one of the added clauses and its own row here fails.
func TestARecordThatIsOverAuthorizesNothing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spoil func(t *testing.T, e *resolveEnv, company, invoice ids.UUID)
	}{
		{"voided invoice", func(t *testing.T, e *resolveEnv, _, invoice ids.UUID) {
			if _, err := e.owner.Exec(context.Background(),
				`UPDATE finance_invoice SET status = 'void', void_at = now() WHERE id = $1`, invoice); err != nil {
				t.Fatal(err)
			}
		}},
		{"archived invoice", func(t *testing.T, e *resolveEnv, _, invoice ids.UUID) {
			if _, err := e.owner.Exec(context.Background(),
				`UPDATE finance_invoice SET archived_at = now() WHERE id = $1`, invoice); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupResolve(t)
			company := e.company(t)
			invoice := e.invoice(t, company, false)
			e.billTo(t, company)
			tc.spoil(t, e, company, invoice)

			got := e.resolve(t, commsauthz.Request{
				Context:  commsauthz.CategoryInvoiceOrPayment,
				Evidence: commsauthz.Evidence{InvoiceID: invoice},
			})

			if got.Supported {
				t.Fatalf("a %s still authorized a message about it", tc.name)
			}
		})
	}
}

// THE GROUND IS RECORDED ONCE, AT STAGING. The transmit phase re-checks the
// evidence but writes nothing: communication_decision stores no anchor, so a
// transmit-phase basis would be UNSCOPED and would match every other unscoped
// row, collapsing the thread separation staging established.
//
// Mutation: drop the phase guard in decideResolved and this fails.
func TestTheGroundIsRecordedAtStagingAndNotAgainAtTransmit(t *testing.T) {
	e := setupResolve(t)
	anchor := e.inboundFrom(t, "thread-1", e.address, time.Now().Add(-time.Hour))
	req := commsauthz.Request{AnchorActivityID: anchor}

	e.decide(t, req)
	// The same recipient decided again at TRANSMIT. The request carries a
	// recent inbound to reach a SUPPORTED resolution — otherwise nothing would
	// try to record a ground and the phase guard would go untested — but no
	// anchor, which is what the transmit phase really has.
	var out commsauthz.Decision
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		out, err = e.gate.decideOne(e.ctx, tx, connector.Recipient{Email: e.address},
			commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup},
			commsauthz.PhaseTransmit)
		return err
	}); err != nil {
		t.Fatalf("deciding at transmit: %v", err)
	}
	if out.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("the transmit decision was %q, want a supported allow so the phase guard is what is under test", out.Verdict)
	}

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_basis WHERE contact_id = $1`, e.contact).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("staging and transmit wrote %d basis rows, want the staging one alone", rows)
	}
	var threadKey *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT thread_key FROM communication_basis WHERE contact_id = $1`, e.contact).Scan(&threadKey); err != nil {
		t.Fatal(err)
	}
	if threadKey == nil || *threadKey != "thread-1" {
		t.Fatal("the recorded ground lost the conversation it was earned on")
	}
}

// dropGrant removes one object grant from the env's principal, so a test can
// ask what a seat WITHOUT it sees.
func (e *resolveEnv) dropGrant(t *testing.T, object string) {
	t.Helper()
	actor, ok := principal.Actor(e.ctx)
	if !ok {
		t.Fatal("the env has no actor to narrow")
	}
	narrowed := map[string]principal.ObjectGrant{}
	for name, grant := range actor.Permissions.Objects {
		if name != object {
			narrowed[name] = grant
		}
	}
	actor.Permissions.Objects = narrowed
	e.ctx = principal.WithActor(e.ctx, actor)
}

// A STOP THAT DOES NOT REACH THIS MESSAGE LEAVES ITS GROUND ON THE RECORD.
//
// The basis write asked whether the subject carried any live suppression at
// all, which is a different question from whether one binds THIS send. An
// objection to direct marketing does not reach a reply on a thread the subject
// started: the reply goes out, lawfully, and the ground it went out on was not
// written down. Their own Art. 15 export then answered "we relied on nothing"
// for a send that was in fact lawful — the exact gap recordBasis exists to
// close, reopened one condition to the left.
//
// The pair is the point. The objection binds nothing here and the ground is
// recorded; the restriction binds the same message and it is not. One case
// alone would pass against a writer that had stopped consulting suppression at
// all, which is the failure on the other side of this one.
func TestAStopThatBindsNothingHereStillLeavesTheGroundRecorded(t *testing.T) {
	e := setupResolve(t)
	e.suppress(t, commsauthz.ReasonObjection)
	anchor := e.inboundFrom(t, "thread-1", e.address, time.Now().Add(-time.Hour))

	if d := e.decide(t, commsauthz.Request{AnchorActivityID: anchor}); d.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("a reply on a thread the subject started was refused by an objection to "+
			"marketing, which does not reach it: %q", d.ReasonCode)
	}

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_basis WHERE contact_id = $1`, e.contact).Scan(&rows); err != nil {
		t.Fatalf("reading the recorded ground: %v", err)
	}
	if rows != 1 {
		t.Fatalf("the allowed send recorded %d grounds, want 1 — a live objection that binds no "+
			"part of this message is not a reason to leave the subject's export saying we relied "+
			"on nothing", rows)
	}
}

// The other side of it: a stop that DOES bind leaves no basis behind.
//
// A restriction reaches a reply — Art. 18(2) leaves no room for it — so the
// send is refused and nothing about a lawful ground is written. A basis row
// here would be a claim made after the subject said stop, in their own export.
func TestAStopThatBindsThisMessageWritesNoGround(t *testing.T) {
	e := setupResolve(t)
	// The STORED kind, which is what the table's own check constraint admits;
	// liveSuppression maps it onto commsauthz.ReasonRestricted on the way in.
	e.suppress(t, "processing_restriction")
	anchor := e.inboundFrom(t, "thread-1", e.address, time.Now().Add(-time.Hour))

	if d := e.decide(t, commsauthz.Request{AnchorActivityID: anchor}); d.Verdict != commsauthz.VerdictDeny {
		t.Fatalf("a restriction did not refuse the reply: %q", d.Verdict)
	}

	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_basis WHERE contact_id = $1`, e.contact).Scan(&rows); err != nil {
		t.Fatalf("reading the recorded ground: %v", err)
	}
	if rows != 0 {
		t.Errorf("a restricted subject carries %d recorded ground(s) — writing one is itself "+
			"processing, and it lands in their export as a claim made after they said stop", rows)
	}
}

// A contract that is not in force carries neither a contract notice nor a
// customer's service mail (contractCarriesMailSQL). A status of expired or
// cancelled refuses even when no date says so.
//
// Mutation: put back a bare archived_at check in either validator and its
// half of these cases fails.
func TestAContractNotInForceCarriesNothing(t *testing.T) {
	for _, tc := range []struct{ name, spoil string }{
		{"draft", `UPDATE contract SET status = 'draft' WHERE id = $1`},
		{"expired", `UPDATE contract SET ends_on = current_date - 1 WHERE id = $1`},
		{"cancelled and effective", `UPDATE contract SET cancellation_effective_on = current_date - 1 WHERE id = $1`},
		{"superseded", `UPDATE contract SET status = 'superseded', superseded_by_id = $2 WHERE id = $1`},
		{"not yet started", `UPDATE contract SET starts_on = current_date + 30 WHERE id = $1`},
		{"status expired with no dates", `UPDATE contract SET status = 'expired', ends_on = NULL WHERE id = $1`},
		{"status cancelled with no dates", `UPDATE contract SET status = 'cancelled', cancellation_effective_on = NULL WHERE id = $1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupResolve(t)
			company := e.company(t)
			e.employ(t, company)
			e.billTo(t, company)
			contract := e.contract(t, company, nil)
			successor := e.contract(t, e.company(t), nil)
			args := []any{contract}
			if strings.Contains(tc.spoil, "$2") {
				args = append(args, successor)
			}
			if _, err := e.owner.Exec(context.Background(), tc.spoil, args...); err != nil {
				t.Fatalf("spoiling the contract: %v", err)
			}

			if got := e.resolve(t, commsauthz.Request{
				Context: commsauthz.CategoryContractNotice, Evidence: commsauthz.Evidence{ContractID: contract},
			}); got.Supported {
				t.Error("a contract notice went out on a contract not in force")
			}
			if got := e.resolve(t, commsauthz.Request{Context: commsauthz.CategoryCustomerService}); got.Supported {
				t.Error("service mail went out on a contract not in force")
			}
		})
	}
}

// A cancelled contract still carries mail until its cancellation takes effect.
//
// Mutation: drop the cancelled arm from contractCarriesMailSQL and this fails.
func TestACancelledContractCarriesMailUntilTheCancellationTakesEffect(t *testing.T) {
	e := setupResolve(t)
	company := e.company(t)
	e.employ(t, company)
	e.billTo(t, company)
	contract := e.contract(t, company, nil)
	if _, err := e.owner.Exec(context.Background(), `
		UPDATE contract SET status = 'cancelled', cancellation_notice_on = current_date,
		       cancellation_effective_on = current_date + 30 WHERE id = $1`, contract); err != nil {
		t.Fatalf("cancelling the contract: %v", err)
	}
	if got := e.resolve(t, commsauthz.Request{
		Context: commsauthz.CategoryContractNotice, Evidence: commsauthz.Evidence{ContractID: contract},
	}); !got.Supported {
		t.Errorf("a contract notice was refused before the cancellation took effect: %q", got.Reason)
	}
}
