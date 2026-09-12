// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// What the one-click unsubscribe endpoint stops, and what it says it
// stopped. Both were wrong in ways no unit test could see: one reported a
// change that never happened, the other skipped the exact lane the link
// was sent under.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pressUnsubscribe drives the endpoint the way a mailbox provider or the
// product's own page does, and returns the purposes it reports changing.
func pressUnsubscribe(t *testing.T, e *channelConsentEnv, token string, purpose *string, body string) []string {
	t.Helper()
	h := NewHandlers(database.BindTo(e.store.db.Pool(), ids.From[ids.WorkspaceKind](e.ws)))
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   "system:public_preferences",
	})
	req := httptest.NewRequest(http.MethodPost,
		"/v1/public/preferences/"+token+"/unsubscribe", strings.NewReader(body)).WithContext(ctx)
	if body != "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	h.OneClickUnsubscribe(rec, req, token, crmcontracts.OneClickUnsubscribeParams{Purpose: purpose})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Unsubscribed []string `json:"unsubscribed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode unsubscribe response: %v", err)
	}
	return out.Unsubscribed
}

// A second press is not a second unsubscribe, and the answer has to say
// so: the page shows "you are already unsubscribed" on an empty array,
// and the old handler echoed the purpose back either way — so a reader
// who pressed twice got a fresh confirmation for a no-op.
func TestAReplayedUnsubscribeReportsNothingChanged(t *testing.T) {
	e := setupChannelConsent(t)
	token := seedPreferenceToken(t, e)
	key := "newsletter"

	first := pressUnsubscribe(t, e, token, &key, "List-Unsubscribe=One-Click")
	if len(first) != 1 || first[0] != key {
		t.Fatalf("first press = %v, want [%s]", first, key)
	}
	second := pressUnsubscribe(t, e, token, &key, "List-Unsubscribe=One-Click")
	if len(second) != 0 {
		t.Errorf("replay = %v, want [] — nothing moved the second time", second)
	}
	if got := consentStateOf(t, e, e.newsletter); got != string(StateWithdrawn) {
		t.Errorf("newsletter = %q, want withdrawn — the replay must not undo it", got)
	}
}

// TestUnsubscribeAllStopsAMarketingLaneWithNoGrantedRow is the incident's own
// case, kept and re-aimed.
//
// The incident: an unsubscribe-all that filtered on 'granted' walked past a
// lane with no row at all, while reporting success. No row is exactly the state
// a contact is in when they have simply been written to, so filtering on their
// stored state meant the press missed the lane the mail was going out under.
// The fix was to let the CATALOG decide rather than the recipient's state, and
// that is what this still guards.
//
// WHAT REVERSED, and why it is not a weakening. The original test used
// business_correspondence as its unrowed lane and asserted the press stopped
// it. An unsubscribe no longer reaches correspondence: nobody subscribed to it,
// so there was nothing there to withdraw, and sweeping it meant a contact who
// unsubscribed from a newsletter stopped receiving the replies to their own
// enquiries. The scope moved; the defect this test names did not, so it is
// asked here about a MARKETING lane with no row — which is the same shape of
// mistake inside the new boundary.
//
// A subject who wants correspondence stopped has a route, and it is a different
// legal act: PublicStop's Art. 21 objection, tested in publicstop_integration_test.go.
func TestUnsubscribeAllStopsAMarketingLaneWithNoGrantedRow(t *testing.T) {
	e := setupChannelConsent(t)
	token := seedPreferenceToken(t, e)

	// Seeded HERE with its real class, because the shared env inserts its
	// purposes by hand and every one of them lands on the 'marketing'
	// default — so the lane this test is about does not otherwise exist,
	// and a test that read it from the env would be testing a marketing
	// purpose under a business name.
	unrowed := ids.From[ids.PurposeKind](ids.NewV7())
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO consent_purpose (id, key, label, requires_double_opt_in, class)
		 VALUES ($1, 'campaign_blast', 'Campaign blast', false, $2)`,
		unrowed, ClassMarketing); err != nil {
		t.Fatalf("seed the campaign purpose: %v", err)
	}
	// Nothing is seeded for it on purpose: no row at all is exactly the
	// state a contact is in when they have simply been written to.
	if got := consentStateOf(t, e, unrowed); got != "" {
		t.Fatalf("precondition: campaign_blast = %q, want no row", got)
	}

	stopped := pressUnsubscribe(t, e, token, nil, "")

	var sawCampaign bool
	for _, key := range stopped {
		if key == "campaign_blast" {
			sawCampaign = true
		}
	}
	if !sawCampaign {
		t.Errorf("unsubscribe-all reported %v — it must stop the lane the mail was sent "+
			"under, and a lane with no row is exactly the one a state filter walks past", stopped)
	}
	if got := consentStateOf(t, e, unrowed); got != string(StateWithdrawn) {
		t.Errorf("campaign_blast = %q, want withdrawn", got)
	}
}

// TestUnsubscribeAllLeavesBusinessCorrespondenceAlone is the other half, and it
// is what the press reversing cost and gained.
//
// Nobody subscribed to correspondence, so an unsubscribe has nothing there to
// withdraw. Sweeping it meant a contact who unsubscribed from a newsletter
// stopped receiving the replies to their own enquiries — recorded, on top of
// that, as a withdrawal of a consent that was never the basis for those
// messages.
func TestUnsubscribeAllLeavesBusinessCorrespondenceAlone(t *testing.T) {
	e := setupChannelConsent(t)
	token := seedPreferenceToken(t, e)

	business := ids.From[ids.PurposeKind](ids.NewV7())
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO consent_purpose (id, key, label, requires_double_opt_in, class)
		 VALUES ($1, 'business_correspondence', 'Business correspondence', false, $2)`,
		business, ClassBusinessCorrespondence); err != nil {
		t.Fatalf("seed the business_correspondence purpose: %v", err)
	}

	for _, key := range pressUnsubscribe(t, e, token, nil, "") {
		if key == "business_correspondence" {
			t.Fatal("unsubscribe-all withdrew business correspondence — the contact who " +
				"unsubscribed from a newsletter stops receiving replies to their own enquiries")
		}
	}
	if got := consentStateOf(t, e, business); got == string(StateWithdrawn) {
		t.Error("business correspondence was withdrawn by an unsubscribe")
	}
}

// Transactional is never withdrawable, and an unsubscribe-all must not
// try: the reader keeps the password-reset mail they will need.
func TestUnsubscribeAllLeavesTransactionalAlone(t *testing.T) {
	e := setupChannelConsent(t)
	token := seedPreferenceToken(t, e)

	for _, key := range pressUnsubscribe(t, e, token, nil, "") {
		if key == PurposeTransactional {
			t.Fatalf("unsubscribe-all withdrew %q", key)
		}
	}
	var state string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT coalesce(max(pc.state), '') FROM contact_consent pc
		   JOIN consent_purpose cp ON cp.id = pc.purpose_id
		  WHERE pc.contact_id = $1 AND cp.key = $2`, e.contact, PurposeTransactional).Scan(&state); err != nil {
		t.Fatalf("read transactional state: %v", err)
	}
	if state == string(StateWithdrawn) {
		t.Error("transactional was withdrawn from the preference centre")
	}
}

// A grant the subject cannot take is reported as a refusal, and a purpose
// that genuinely is not there is NOT: the two both answer ErrNotFound
// inside the write, and collapsing them would tell a recipient their
// choice was declined when in truth nothing looked at it.
func TestAnUnknownPurposeIsAFaultNotARefusal(t *testing.T) {
	e := setupChannelConsent(t)
	token := seedPreferenceToken(t, e)

	rec := preferenceSave(t, e, token,
		`{"choices":[{"purpose_key":"no_such_purpose","state":"granted"}]}`)

	if rec.Code == http.StatusOK {
		t.Errorf("status = %d — a purpose the catalog does not carry must not read as an ordinary refusal: %s",
			rec.Code, rec.Body.String())
	}
}
