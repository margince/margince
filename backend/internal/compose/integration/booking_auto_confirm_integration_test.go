// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// tickEnv drives the public booking form with a marketing tick, one slot per call.
type tickEnv struct {
	e            *apptest.AppEnv
	base         string
	transact     string
	marketing    string
	nextSlotHour time.Duration
}

func newTickEnv(t *testing.T) *tickEnv {
	t.Helper()
	e := setupBookingApp(t)
	e.BootstrapWorkspace(t)
	apptest.EnableGoogleBookingPage(t, e)
	return &tickEnv{
		e: e, base: "/v1/public/booking/" + bookingSlug(t, e),
		transact: seededTransactionalPurposeID(t, e), marketing: seededMarketingPurposeID(t, e),
		nextSlotHour: 2 * time.Hour,
	}
}

// book posts a booking with a marketing tick and returns the marketing outcome.
func (x *tickEnv) book(t *testing.T, email string) string {
	t.Helper()
	start := nextMonday().Add(x.nextSlotHour)
	x.nextSlotHour += 30 * time.Minute
	body := AnyMap{
		"start": start, "end": start.Add(30 * time.Minute),
		"booker": AnyMap{"name": "Mia Mailed", "email": email},
		"consent": AnyMap{
			"purpose_id": x.transact, "policy_version": "pp-2026-01",
			"wording": "You agree we may contact you about this meeting.",
			"marketing": AnyMap{
				"purpose_id": x.marketing, "policy_version": "mk-2026-01",
				"wording": "Send me your newsletter.",
			},
		},
	}
	var answer struct {
		Marketing string `json:"marketing"`
	}
	if status := publicCall(t, x.e, "POST", x.base, body, nil, &answer); status != http.StatusCreated {
		t.Fatalf("a booking carrying a marketing tick → %d, want 201", status)
	}
	return answer.Marketing
}

// counts reads the tokens and the staged confirmation mails held for one address.
func (x *tickEnv) counts(t *testing.T, email string) (tokens, mails int) {
	t.Helper()
	if err := x.e.Owner.QueryRow(context.Background(), `
		SELECT count(*), count(*) FILTER (WHERE EXISTS (SELECT 1 FROM comms_outbound o WHERE o.link_id = ct.id))
		  FROM confirm_token ct WHERE lower(ct.delivered_to) = lower($1)`, email).Scan(&tokens, &mails); err != nil {
		t.Fatal(err)
	}
	return tokens, mails
}

func (x *tickEnv) wantQuiet(t *testing.T, email, outcome string, tokens, mails int) {
	t.Helper()
	if outcome != "pending_confirmation" {
		t.Errorf("outcome = %q, want the same neutral answer for every address", outcome)
	}
	if gotT, gotM := x.counts(t, email); gotT != tokens || gotM != mails {
		t.Errorf("tokens=%d mails=%d, want tokens=%d mails=%d", gotT, gotM, tokens, mails)
	}
}

// ageLinks moves every link back in time, standing in for a clock that moved on.
func (x *tickEnv) ageLinks(t *testing.T, days int) {
	t.Helper()
	if _, err := x.e.Owner.Exec(context.Background(), `
		UPDATE confirm_token SET issued_at = issued_at - make_interval(days => $1),
		                         expires_at = expires_at - make_interval(days => $1)`, days); err != nil {
		t.Fatal(err)
	}
}

func (x *tickEnv) consentState(t *testing.T, email string) string {
	t.Helper()
	var state string
	err := x.e.Owner.QueryRow(context.Background(), `
		SELECT coalesce((SELECT cc.state FROM contact_consent cc WHERE cc.contact_id = ce.contact_id AND cc.purpose_id = $2), 'none')
		  FROM contact_email ce WHERE ce.email = $1`, email, x.marketing).Scan(&state)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestAMarketingTickMailsOneConfirmationAndARepeatDoesNotMailAgain(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "mia@visitor.example"
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	assertNoMarketingGrant(t, x.e, contactIDByEmail(t, x.e, email), x.marketing)

	var read struct {
		State []struct {
			PurposeID    string     `json:"purpose_id"`
			State        string     `json:"state"`
			PendingUntil *time.Time `json:"confirmation_pending_until"`
		} `json:"state"`
	}
	if status := x.e.Call(t, "GET", "/v1/contacts/"+contactIDByEmail(t, x.e, email)+"/consent", nil, nil, &read); status != http.StatusOK {
		t.Fatalf("reading the consent state → %d", status)
	}
	for _, st := range read.State {
		if st.PurposeID == x.marketing && (st.State != "unknown" || st.PendingUntil == nil) {
			t.Errorf("marketing row reads state=%q pending_until=%v, want unknown with a pending expiry", st.State, st.PendingUntil)
		}
	}

	x.wantQuiet(t, email, x.book(t, email), 1, 1)
}

func TestTheConfirmationBudgetPerAddressIsOnePerWeekAndThreePerQuarter(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "budget@visitor.example"
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	x.ageLinks(t, 1)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	x.ageLinks(t, 7)
	x.wantQuiet(t, email, x.book(t, email), 2, 2)
	x.ageLinks(t, 8)
	x.wantQuiet(t, email, x.book(t, email), 3, 3)
	x.ageLinks(t, 15)
	// A fourth within 90 days: the address spent its budget, nothing is mailed,
	// and the answer is still the neutral one.
	x.wantQuiet(t, email, x.book(t, email), 3, 3)
	assertNoMarketingGrant(t, x.e, contactIDByEmail(t, x.e, email), x.marketing)
}

func TestAnErasedAddressIsNotMailedAConfirmation(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "erased@visitor.example"
	if _, err := x.e.Owner.Exec(context.Background(),
		`INSERT INTO erasure_suppression (kind, value_hash) VALUES ('email', $1) ON CONFLICT DO NOTHING`,
		storekit.SuppressionHash(t.Context(), email)); err != nil {
		t.Fatal(err)
	}
	x.wantQuiet(t, email, x.book(t, email), 0, 0)
	if got := x.consentState(t, email); got == "granted" {
		t.Errorf("an erased address holds marketing consent %q", got)
	}
}

func TestAContactWhoAlreadySubscribedIsNotMailedAgain(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "subscribed@visitor.example"
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	grantWithServedQuestion(t, x.e, confirmLinkToken(t, x.e))
	if got := x.consentState(t, email); got != "granted" {
		t.Fatalf("consent after spending the link = %q, want granted", got)
	}
	x.ageLinks(t, 30)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	if got := x.consentState(t, email); got != "granted" {
		t.Errorf("consent after the second booking = %q, want granted", got)
	}
}

func TestAStoppedContactIsNotMailedAConfirmation(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"marketing_objection", "processing_restriction", "subject_request"} {
		t.Run(kind, func(t *testing.T) {
			x := newTickEnv(t)
			email := kind + "@visitor.example"
			var contact struct {
				ID string `json:"id"`
			}
			if status := x.e.Call(t, "POST", "/v1/contacts", AnyMap{
				"source": "manual", "full_name": "Stopped Contact",
				"emails": []AnyMap{{"email": email, "email_type": "work", "is_primary": true}},
			}, nil, &contact); status != http.StatusCreated {
				t.Fatalf("create contact → %d", status)
			}
			if status := x.e.Call(t, "POST", "/v1/contacts/"+contact.ID+"/consent/suppress",
				AnyMap{"kind": kind, "reason": "The recipient asked us to stop."}, nil, nil); status != http.StatusNoContent {
				t.Fatalf("recording the stop → %d", status)
			}
			x.wantQuiet(t, email, x.book(t, email), 0, 0)
		})
	}
}

// leadTick creates a lead holding email and returns its id.
func (x *tickEnv) leadTick(t *testing.T, email string) string {
	t.Helper()
	var lead struct {
		ID string `json:"id"`
	}
	if status := x.e.Call(t, "POST", "/v1/leads", AnyMap{
		"full_name": "Lena Lead", "email": email, "source": "manual",
	}, nil, &lead); status != http.StatusCreated {
		t.Fatalf("create lead → %d", status)
	}
	return lead.ID
}

func (x *tickEnv) leadConsentState(t *testing.T, leadID string) string {
	t.Helper()
	var state string
	if err := x.e.Owner.QueryRow(context.Background(), `
		SELECT coalesce((SELECT state FROM contact_consent WHERE lead_id = $1 AND purpose_id = $2), 'none')`,
		leadID, x.marketing).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestATickOnABookingMatchedToALeadMailsOneConfirmationToItsAddress(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "lead@visitor.example"
	leadID := x.leadTick(t, email)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)

	var contactSet bool
	var kind string
	if err := x.e.Owner.QueryRow(context.Background(), `
		SELECT contact_id IS NOT NULL, kind FROM confirm_token WHERE lead_id = $1`, leadID).Scan(&contactSet, &kind); err != nil {
		t.Fatalf("the lead holds no link of its own: %v", err)
	}
	if contactSet || kind != "consent_confirmation" {
		t.Errorf("link has contact=%v kind=%q, want a consent link for the lead alone", contactSet, kind)
	}
	if got := x.leadConsentState(t, leadID); got != "none" {
		t.Errorf("the tick alone left the lead's marketing state %q, want none until the link is opened", got)
	}
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
}

// leadWrites counts the audit rows and consent.changed events that name the lead.
func (x *tickEnv) leadWrites(t *testing.T, leadID string) (audits, events int) {
	t.Helper()
	if err := x.e.Owner.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM audit_log WHERE action = 'consent_grant' AND entity_id = $1),
		       (SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'consent.changed' AND envelope::text LIKE '%'||$1||'%')`,
		leadID).Scan(&audits, &events); err != nil {
		t.Fatal(err)
	}
	return audits, events
}

func TestOpeningALeadsConfirmationGrantsItsMarketingConsentWithTheWriteShape(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "lead.opens@visitor.example"
	leadID := x.leadTick(t, email)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	auditsBefore, eventsBefore := x.leadWrites(t, leadID)

	grantWithServedQuestion(t, x.e, confirmLinkToken(t, x.e))

	if got := x.leadConsentState(t, leadID); got != "granted" {
		t.Fatalf("lead marketing state after the link = %q, want granted", got)
	}
	var proofs int
	if err := x.e.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM consent_event WHERE lead_id = $1 AND contact_id IS NULL AND purpose_id = $2
		   AND new_state = 'granted' AND double_opt_in_confirmed_at IS NOT NULL AND issuance_trigger IS NOT NULL`,
		leadID, x.marketing).Scan(&proofs); err != nil {
		t.Fatal(err)
	}
	audits, events := x.leadWrites(t, leadID)
	if proofs != 1 || audits != auditsBefore+1 || events != eventsBefore+1 {
		t.Errorf("proof rows=%d new audit rows=%d new consent.changed events=%d, want 1 of each",
			proofs, audits-auditsBefore, events-eventsBefore)
	}
}

func TestAPromotedLeadKeepsTheMarketingGrantItsLinkEarned(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "lead.promoted@visitor.example"
	leadID := x.leadTick(t, email)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	grantWithServedQuestion(t, x.e, confirmLinkToken(t, x.e))

	var promoted struct {
		Contact struct {
			ID string `json:"id"`
		} `json:"contact"`
	}
	if status := x.e.Call(t, "POST", "/v1/leads/"+leadID+"/promote",
		AnyMap{"trigger": "human_qualify"}, nil, &promoted); status != http.StatusOK || promoted.Contact.ID == "" {
		t.Fatalf("promote → %d %+v", status, promoted)
	}
	var state string
	if err := x.e.Owner.QueryRow(context.Background(), `
		SELECT coalesce((SELECT state FROM contact_consent WHERE contact_id = $1 AND purpose_id = $2), 'none')`,
		promoted.Contact.ID, x.marketing).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "granted" {
		t.Errorf("the promoted contact's marketing state = %q, want the lead's grant carried", state)
	}
	var proofs int
	if err := x.e.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM consent_event WHERE contact_id = $1 AND purpose_id = $2
		   AND double_opt_in_confirmed_at IS NOT NULL AND issuance_trigger IS NOT NULL`,
		promoted.Contact.ID, x.marketing).Scan(&proofs); err != nil {
		t.Fatal(err)
	}
	if proofs != 1 {
		t.Errorf("the promoted contact holds %d confirmed proofs, want the lead's copied so the grant authorizes mail", proofs)
	}
	x.ageLinks(t, 30)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
}

func TestAnErasedLeadAddressIsNotMailedAConfirmation(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "erased.lead@visitor.example"
	leadID := x.leadTick(t, email)
	if _, err := x.e.Owner.Exec(context.Background(),
		`INSERT INTO erasure_suppression (kind, value_hash) VALUES ('email', $1) ON CONFLICT DO NOTHING`,
		storekit.SuppressionHash(t.Context(), email)); err != nil {
		t.Fatal(err)
	}
	x.wantQuiet(t, email, x.book(t, email), 0, 0)
	if got := x.leadConsentState(t, leadID); got == "granted" {
		t.Errorf("an erased address holds marketing consent %q", got)
	}
}

func TestAStoppedLeadIsNotMailedAConfirmation(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"marketing_objection", "processing_restriction", "subject_request"} {
		t.Run(kind, func(t *testing.T) {
			x := newTickEnv(t)
			email := kind + ".lead@visitor.example"
			leadID := x.leadTick(t, email)
			if _, err := x.e.Owner.Exec(context.Background(), `
				INSERT INTO communication_suppression (lead_id, kind, captured_by, decided_by_level, source)
				VALUES ($1, $2, 'test', 'user', 'test')`, leadID, kind); err != nil {
				t.Fatal(err)
			}
			x.wantQuiet(t, email, x.book(t, email), 0, 0)
		})
	}
}

func TestALeadWhoAlreadySubscribedIsNotMailedAgain(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "lead.subscribed@visitor.example"
	leadID := x.leadTick(t, email)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	grantWithServedQuestion(t, x.e, confirmLinkToken(t, x.e))
	x.ageLinks(t, 30)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	if got := x.leadConsentState(t, leadID); got != "granted" {
		t.Errorf("lead consent after the second booking = %q, want granted", got)
	}
}

func TestALeadWhoWithdrewIsNotAskedAgainByABooking(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "lead.withdrew@visitor.example"
	leadID := x.leadTick(t, email)
	if _, err := x.e.Owner.Exec(context.Background(), `
		INSERT INTO contact_consent (lead_id, purpose_id, state, lawful_basis, captured_at, source)
		VALUES ($1, $2, 'withdrawn', 'consent', now(), 'test')`, leadID, x.marketing); err != nil {
		t.Fatal(err)
	}
	x.wantQuiet(t, email, x.book(t, email), 0, 0)
}

func TestTheConfirmationBudgetPerAddressHoldsForALead(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "lead.budget@visitor.example"
	x.leadTick(t, email)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	x.ageLinks(t, 1)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	x.ageLinks(t, 7)
	x.wantQuiet(t, email, x.book(t, email), 2, 2)
	x.ageLinks(t, 8)
	x.wantQuiet(t, email, x.book(t, email), 3, 3)
	x.ageLinks(t, 15)
	x.wantQuiet(t, email, x.book(t, email), 3, 3)
}

func TestAHardBouncedAddressIsNotMailedAConfirmationToALead(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "bounced.lead@visitor.example"
	leadID := x.leadTick(t, email)
	if _, err := x.e.Owner.Exec(context.Background(), `
		INSERT INTO communication_suppression (address, kind, source, captured_by, decided_by_level, recorded_through)
		VALUES ($1, 'hard_bounce', 'delivery_report', 'system:comms', 'machine', 'machine')`, email); err != nil {
		t.Fatal(err)
	}
	x.wantQuiet(t, email, x.book(t, email), 0, 0)
	if got := x.leadConsentState(t, leadID); got == "granted" {
		t.Errorf("a bounced address holds marketing consent %q", got)
	}
}

func TestAContactAndALeadOnOneAddressShareTheConfirmationBudget(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "shared@visitor.example"
	x.leadTick(t, email)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)

	// The API refuses a second record on a held address. The contact sharing
	// it is written directly, as an import predating the lead would.
	contactID := ids.NewV7()
	if _, err := x.e.Owner.Exec(context.Background(),
		`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Shared Contact', 'manual', 'human:x')`,
		contactID); err != nil {
		t.Fatal(err)
	}
	if _, err := x.e.Owner.Exec(context.Background(),
		`INSERT INTO contact_email (contact_id, email, email_type, is_primary, source, captured_by) VALUES ($1, $2, 'work', true, 'manual', 'human:x')`,
		contactID, email); err != nil {
		t.Fatal(err)
	}
	x.ageLinks(t, 1)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	x.ageLinks(t, 7)
	x.wantQuiet(t, email, x.book(t, email), 2, 2)
	var contactLinks int
	if err := x.e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM confirm_token WHERE contact_id = $1`, contactID).Scan(&contactLinks); err != nil {
		t.Fatal(err)
	}
	if contactLinks != 1 {
		t.Errorf("the contact holds %d links, want the second one", contactLinks)
	}
	x.ageLinks(t, 8)
	x.wantQuiet(t, email, x.book(t, email), 3, 3)
	x.ageLinks(t, 15)
	x.wantQuiet(t, email, x.book(t, email), 3, 3)
}

func TestALeadsLinkIsDeadOnceTheLeadIsPromoted(t *testing.T) {
	t.Parallel()
	x := newTickEnv(t)
	email := "lead.dead.link@visitor.example"
	leadID := x.leadTick(t, email)
	x.wantQuiet(t, email, x.book(t, email), 1, 1)
	token := confirmLinkToken(t, x.e)
	question := servedQuestion(t, x.e, token).Text

	var promoted struct {
		Contact struct {
			ID string `json:"id"`
		} `json:"contact"`
	}
	if status := x.e.Call(t, "POST", "/v1/leads/"+leadID+"/promote",
		AnyMap{"trigger": "human_qualify"}, nil, &promoted); status != http.StatusOK || promoted.Contact.ID == "" {
		t.Fatalf("promote → %d %+v", status, promoted)
	}
	if status := publicCall(t, x.e, "GET", "/v1/public/confirm/"+token, nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("opening the promoted lead's link → %d, want 404", status)
	}
	if status := publicCall(t, x.e, "POST", "/v1/public/confirm/"+token, AnyMap{
		"marketing_choice": "granted", "marketing_wording": question,
	}, nil, nil); status != http.StatusNotFound {
		t.Errorf("answering the promoted lead's link → %d, want 404", status)
	}
	var grants int
	if err := x.e.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM contact_consent WHERE purpose_id = $3 AND state = 'granted'
		   AND (lead_id = $1 OR contact_id = $2)`, leadID, promoted.Contact.ID, x.marketing).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 0 {
		t.Errorf("a dead link left %d marketing grants on the lead or the contact", grants)
	}
}
