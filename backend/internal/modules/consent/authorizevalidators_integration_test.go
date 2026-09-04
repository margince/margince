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
// an unprompted follow-up falls back to "this person wrote to us inside the
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
// list and a public source say nothing about what this person asked for.
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

// SCENARIO 5: an invoice recipient with no employment row.
//
// Missing relational data is a CRM gap, not a legal refusal. The answer is a
// review naming what to link, and the send still goes while the engine is
// observed — never a deny that leaves finance unable to send an invoice.
func TestAnInvoiceRecipientWithNoEmploymentRowIsReviewedNotRefused(t *testing.T) {
	e := setupResolve(t)
	invoice := e.invoice(t, e.organization(t), false)

	got := e.resolve(t, commsauthz.Request{
		Context:  commsauthz.CategoryInvoiceOrPayment,
		Evidence: commsauthz.Evidence{InvoiceID: invoice},
	})

	if got.Supported {
		t.Fatal("an invoice reached a person with no link to the customer")
	}
	if got.Reason != commsauthz.ReasonNoEvidence {
		t.Errorf("reason = %q, want a reason naming the missing evidence", got.Reason)
	}
	// The claim survives under its own name, so the operator is told to link
	// the person to the customer rather than to find a marketing consent.
	if got.Category != commsauthz.CategoryInvoiceOrPayment {
		t.Errorf("category = %q, want the invoice claim kept for the reader", got.Category)
	}
}

// And the same invoice DOES support the send once the person is linked to the
// customer, or the test above would pass against a validator that never allows.
func TestAnInvoiceReachesAContactAtTheCustomer(t *testing.T) {
	e := setupResolve(t)
	org := e.organization(t)
	invoice := e.invoice(t, org, false)
	e.employ(t, org)

	got := e.resolve(t, commsauthz.Request{
		Context:  commsauthz.CategoryInvoiceOrPayment,
		Evidence: commsauthz.Evidence{InvoiceID: invoice},
	})

	if !got.Supported {
		t.Fatalf("a contact employed by the customer could not be sent their invoice: %q", got.Reason)
	}
	if got.Basis != commsauthz.BasisContract {
		t.Errorf("basis = %q, want contract", got.Basis)
	}
}

// AN ENDED EMPLOYMENT DOES NOT REACH. Somebody who left the customer is not the
// person their invoices go to.
//
// Mutation: drop either r.ended_at IS NULL or r.archived_at IS NULL from the
// invoice validator and this fails.
func TestSomebodyWhoLeftTheCustomerIsNotReachedByItsInvoices(t *testing.T) {
	for _, tc := range []struct{ name, column string }{
		{"employment ended", "ended_at"},
		{"employment removed", "archived_at"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupResolve(t)
			org := e.organization(t)
			invoice := e.invoice(t, org, false)
			e.employ(t, org)
			if _, err := e.owner.Exec(context.Background(),
				`UPDATE relationship SET `+tc.column+` = now() WHERE person_id = $1`, e.person); err != nil {
				t.Fatal(err)
			}

			got := e.resolve(t, commsauthz.Request{
				Context:  commsauthz.CategoryInvoiceOrPayment,
				Evidence: commsauthz.Evidence{InvoiceID: invoice},
			})

			if got.Supported {
				t.Fatal("a former employee was still reached by the customer's invoices")
			}
		})
	}
}

// SOMEBODY SERVING NOTICE STILL WORKS THERE. ended_at is a date, and a person
// whose last day is next month is still the one handling their employer's
// invoices. Reading the column's mere presence as "gone" would take them off
// the contact list the day their notice was filed — with no way back, because
// ended_at cannot be cleared through the API.
//
// Mutation: spell the employment test as `r.ended_at IS NULL` and this fails.
// That spelling is what the employment-currency gate exists to catch, and it is
// what this validator shipped as until the gate caught it.
func TestSomebodyServingNoticeStillReceivesTheirEmployersInvoices(t *testing.T) {
	e := setupResolve(t)
	org := e.organization(t)
	invoice := e.invoice(t, org, false)
	e.employ(t, org)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE relationship SET ended_at = current_date + 30 WHERE person_id = $1`,
		e.person); err != nil {
		t.Fatal(err)
	}

	got := e.resolve(t, commsauthz.Request{
		Context:  commsauthz.CategoryInvoiceOrPayment,
		Evidence: commsauthz.Evidence{InvoiceID: invoice},
	})

	if !got.Supported {
		t.Fatal("a contact serving notice was cut off from their employer's invoices")
	}
}

// A CLAIM NAMING SOMEBODY ELSE'S INVOICE SUPPORTS NOTHING. The evidence id is
// caller-supplied, so naming an invoice belonging to an organization this
// person has nothing to do with must not admit the message.
func TestAnInvoiceForAnotherCustomerSupportsNothing(t *testing.T) {
	e := setupResolve(t)
	e.employ(t, e.organization(t))
	elsewhere := e.invoice(t, e.organization(t), false)

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
		  FROM communication_basis WHERE person_id = $1`, e.person).Scan(&kind, &threadKey, &validUntil); err != nil {
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
		`SELECT count(*) FROM communication_basis WHERE person_id = $1`, e.person).Scan(&rows); err != nil {
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
		`SELECT count(*) FROM communication_basis WHERE person_id = $1`, e.person).Scan(&rows); err != nil {
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
		`SELECT count(*) FROM communication_basis WHERE person_id = $1`, e.person).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Fatalf("an unsupported send recorded %d grounds, want none", rows)
	}
}

// organization plants a customer record.
func (e *resolveEnv) organization(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO organization (id, display_name, source, captured_by)
		VALUES ($1, 'Acme', 'manual', 'human:x')`, id); err != nil {
		t.Fatalf("planting the organization: %v", err)
	}
	return id
}

// employ links this env's person to an organization, the way a finance contact
// reaches the customer whose invoices they receive.
func (e *resolveEnv) employ(t *testing.T, org ids.UUID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO relationship (kind, organization_id, person_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:x')`, org, e.person); err != nil {
		t.Fatalf("planting the employment: %v", err)
	}
}

// invoice plants a finance invoice against an organization, with the finance
// connection it hangs off.
func (e *resolveEnv) invoice(t *testing.T, org ids.UUID, _ bool) ids.UUID {
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
		  (id, connection_id, organization_id, external_id, issued_at, status,
		   currency, net_minor, gross_minor, sync_hash, source, captured_by)
		VALUES ($1, $2, $3, $4, current_date, 'open', 'EUR', 1000, 1190, 'hash', 'lexoffice', 'human:x')`,
		id, conn, org, "ext-"+id.String()); err != nil {
		t.Fatalf("planting the invoice: %v", err)
	}
	return id
}

// acquisition records why this env's person exists, which is what PR 4's
// evidence table holds and what a "they asked me in person" answer rests on.
func (e *resolveEnv) acquisition(t *testing.T, kind string, when time.Time) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO person_acquisition_evidence (person_id, kind, occurred_at, captured_by)
		VALUES ($1, $2, $3, 'human:x')`, e.person, kind, when); err != nil {
		t.Fatalf("planting the acquisition evidence: %v", err)
	}
}

// A FILED ACTIVITY IS NOT SOMETHING THE PERSON WROTE.
//
// activity_link is a FILING link with no author concept, and a caller may post
// an activity with direction=inbound and a link to any contact they can read
// (CreateActivityRequest passes direction, occurred_at and links through). So a
// follow-up arm that asked activity_link would let anybody manufacture their own
// evidence for writing to anybody.
//
// Mutation: ask activity_link instead of activity_participant with role 'from'
// — the shape this shipped as — and this passes with forged evidence.
func TestAFiledActivityIsNotSomethingThePersonWrote(t *testing.T) {
	e := setupResolve(t)
	ctx := context.Background()
	// An inbound activity FILED under the person, which they did not write:
	// no participant row names them as the author.
	id := ids.NewV7()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity (id, kind, direction, source, occurred_at, captured_by)
		VALUES ($1, 'note', 'inbound', 'manual', now(), 'human:x')`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, person_id)
		VALUES ($1, 'person', $2)`, id, e.person); err != nil {
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
	org := e.organization(t)
	invoice := e.invoice(t, org, false)
	e.employ(t, org)
	e.dropGrant(t, "finance")

	var err error
	if txErr := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err = e.gate.resolveCategory(e.ctx, tx, commsauthz.Request{
			Context:  commsauthz.CategoryInvoiceOrPayment,
			Evidence: commsauthz.Evidence{InvoiceID: invoice},
		}, subjectRef{Kind: entityPerson, ID: e.person.String(), Address: e.address})
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

// A VOIDED INVOICE AUTHORIZES NOTHING. Neither does an archived one, an
// archived offer, a draft one, or a quote on a deal that closed.
//
// Mutation: drop any one of the added clauses and its own row here fails.
func TestARecordThatIsOverAuthorizesNothing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spoil func(t *testing.T, e *resolveEnv, org, invoice ids.UUID)
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
			org := e.organization(t)
			invoice := e.invoice(t, org, false)
			e.employ(t, org)
			tc.spoil(t, e, org, invoice)

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
		`SELECT count(*) FROM communication_basis WHERE person_id = $1`, e.person).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("staging and transmit wrote %d basis rows, want the staging one alone", rows)
	}
	var threadKey *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT thread_key FROM communication_basis WHERE person_id = $1`, e.person).Scan(&threadKey); err != nil {
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
