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
	x := newTickEnv(t)
	email := "erased@visitor.example"
	if _, err := x.e.Owner.Exec(context.Background(),
		`INSERT INTO erasure_suppression (kind, value_hash) VALUES ('email', $1) ON CONFLICT DO NOTHING`,
		storekit.SuppressionHash(email)); err != nil {
		t.Fatal(err)
	}
	x.wantQuiet(t, email, x.book(t, email), 0, 0)
	if got := x.consentState(t, email); got == "granted" {
		t.Errorf("an erased address holds marketing consent %q", got)
	}
}

func TestAContactWhoAlreadySubscribedIsNotMailedAgain(t *testing.T) {
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

func TestATickOnABookingMatchedToALeadMailsNothing(t *testing.T) {
	x := newTickEnv(t)
	email := "lead@visitor.example"
	if status := x.e.Call(t, "POST", "/v1/leads", AnyMap{
		"full_name": "Lena Lead", "email": email, "source": "manual",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("create lead → %d", status)
	}
	x.wantQuiet(t, email, x.book(t, email), 0, 0)
}
