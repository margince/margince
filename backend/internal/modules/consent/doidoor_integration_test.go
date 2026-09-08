// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The double-opt-in door reaches the mint.
//
// IssueConsentLink was built as the honest replacement for the retired operator
// endpoint and then had no production caller at all: the handler refused, and
// the only things reaching the store were two tests. A store method nothing
// calls is a capability the product does not have, however well it is written —
// and the two tests kept it green, so nothing said so.
//
// These go through the HANDLER rather than the store, because the store was
// never the part that was missing.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// issuedBody is what the door answers: two facts about the delivery, and no
// token.
type issuedBody struct {
	DeliveredTo string `json:"delivered_to"`
	Queued      bool   `json:"queued"`
	Sendable    bool   `json:"sendable"`
	ExpiresAt   string `json:"expires_at"`
}

func TestTheDoubleOptInDoorMintsALinkForTheNamedPurpose(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	seedSubjectAddress(t, e)
	purpose := marketingPurposeID(t, e)

	rec := postDoubleOptIn(t, e, purpose)
	if rec.Code != http.StatusCreated {
		t.Fatalf("the door answered %d: %s — it is meant to mint, not refuse",
			rec.Code, rec.Body.String())
	}

	var got issuedBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decoding the answer: %v (%s)", err, rec.Body.String())
	}
	if got.DeliveredTo == "" {
		t.Error("the answer names no address — the subject's own mailbox is what the proof rests on")
	}

	// The row the link actually is, and the purpose it carries. The purpose
	// riding the TOKEN is what stops one link granting a different purpose, so
	// a mint that stored the wrong one would be the whole defect back again.
	var kind string
	var storedPurpose ids.PurposeID
	if err := e.owner.QueryRow(e.ctx,
		`SELECT kind, purpose_id FROM confirm_token
		 WHERE person_id = $1 AND consumed_at IS NULL`, e.person).Scan(&kind, &storedPurpose); err != nil {
		t.Fatalf("reading the minted link: %v", err)
	}
	if kind != LinkConsentConfirmation {
		t.Errorf("the door minted a %q link, want %q", kind, LinkConsentConfirmation)
	}
	if storedPurpose != purpose {
		t.Errorf("the link carries purpose %s, want the one the caller named (%s)", storedPurpose, purpose)
	}
}

// The property the retired endpoint failed: the plaintext never comes back, so
// the operator cannot close the round trip the subject is supposed to close.
func TestTheDoubleOptInDoorReturnsNoPlaintext(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	seedSubjectAddress(t, e)

	rec := postDoubleOptIn(t, e, marketingPurposeID(t, e))
	if rec.Code != http.StatusCreated {
		t.Fatalf("the door answered %d: %s", rec.Code, rec.Body.String())
	}

	// The stored hash is not the token, so a body containing the token cannot be
	// found by comparing against the row. Compare against the SHAPE instead: the
	// answer's four fields are the whole contract, and any other string in it is
	// something nobody declared.
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the answer: %v", err)
	}
	for field := range body {
		switch field {
		case "delivered_to", "expires_at", "queued", "sendable":
		default:
			t.Errorf("the answer carries an undeclared field %q: %s", field, rec.Body.String())
		}
	}
}

// A person with no live address has no mailbox to prove, so there is nothing
// the link could evidence. The door must say so rather than mint against an
// address nobody holds.
func TestTheDoubleOptInDoorRefusesWhenThereIsNoMailbox(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	// Deliberately no seedSubjectAddress.

	rec := postDoubleOptIn(t, e, marketingPurposeID(t, e))
	if rec.Code == http.StatusCreated {
		t.Fatalf("the door minted a link for a person with no address: %s", rec.Body.String())
	}
	var minted int
	if err := e.owner.QueryRow(e.ctx,
		`SELECT count(*) FROM confirm_token WHERE person_id = $1`, e.person).Scan(&minted); err != nil {
		t.Fatalf("counting minted links: %v", err)
	}
	if minted != 0 {
		t.Errorf("%d links were minted for a person with no mailbox", minted)
	}
}

// The mail must ask the question the link was minted for.
//
// The three arms above run with no lane wired, so stageConfirmMail returns at
// its first line and nothing is rendered — which means none of them can see
// templateForLinkKind handing back the RECORD-confirmation wording for a
// consent link. That defect mails somebody "check what we hold about you" and
// records their answer as a marketing grant, and it is invisible without the
// lane.
func TestTheDoubleOptInMailAsksTheConsentQuestion(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	seedSubjectAddress(t, e)
	stager, vault := &recordingStager{}, &recordingVault{}

	rec := postDoubleOptInWith(t, e, withLane(e, stager, vault), marketingPurposeID(t, e))
	if rec.Code != http.StatusCreated {
		t.Fatalf("the door answered %d: %s", rec.Code, rec.Body.String())
	}
	if stager.calls != 1 {
		t.Fatalf("the lane was handed %d messages, want 1 — the mail is what carries the link",
			stager.calls)
	}
	if stager.seen.Rendered.Key != TemplateConsentConfirmation {
		t.Errorf("the mail rendered %q, want %q — a consent link that arrives asking about the "+
			"record asks a question the answer does not fit",
			stager.seen.Rendered.Key, TemplateConsentConfirmation)
	}
	// The category the engine is asked about comes off the template, not a
	// caller, so a consent mail cannot be authorized as something else.
	if stager.seen.Category != commsauthz.CategoryConsentConfirmation {
		t.Errorf("the mail was staged as %q, want %q",
			stager.seen.Category, commsauthz.CategoryConsentConfirmation)
	}
	// The link itself is sealed, never on the row the mail carries.
	if vault.calls != 1 {
		t.Errorf("the vault sealed %d links, want 1", vault.calls)
	}
}

// An archived purpose is nobody's to confirm any more, so a link asking about
// it could only ever arrive dead: consentCardFor resolves the card only for a
// live purpose, and the subject would open a 404 sent in the installation's own
// name. The foreign key does not catch this — an archived row still exists.
func TestTheDoubleOptInDoorRefusesAnArchivedPurpose(t *testing.T) {
	e := setupChannelConsent(t)
	seedMarketingPurpose(t, e)
	seedSubjectAddress(t, e)
	purpose := marketingPurposeID(t, e)
	if _, err := e.owner.Exec(e.ctx,
		`UPDATE consent_purpose SET archived_at = now() WHERE id = $1`, purpose); err != nil {
		t.Fatalf("archiving the purpose: %v", err)
	}

	rec := postDoubleOptIn(t, e, purpose)
	// The SPECIFIC status: "not 201" would also pass on a 500, and a caller
	// cannot act on an internal error the way they can act on being told the
	// purpose is archived.
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("the door answered %d for an archived purpose, want 422: %s",
			rec.Code, rec.Body.String())
	}
	var minted int
	if err := e.owner.QueryRow(e.ctx,
		`SELECT count(*) FROM confirm_token WHERE person_id = $1`, e.person).Scan(&minted); err != nil {
		t.Fatalf("counting minted links: %v", err)
	}
	if minted != 0 {
		t.Errorf("%d links were minted against an archived purpose", minted)
	}
}

// A purpose that needs no double opt-in has nothing for a mailed link to ask.
// Confirming it would record mailbox-proven evidence for a grant that never
// required proof, off the back of a mail the subject was asked to answer for no
// reason. The endpoint's whole subject is the double-opt-in purpose, and the
// contract says so.
func TestTheDoubleOptInDoorRefusesAPurposeThatNeedsNoConfirmation(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	var plain ids.PurposeID
	if err := e.owner.QueryRow(e.ctx,
		`INSERT INTO consent_purpose (key, label, requires_double_opt_in)
		 VALUES ('service_notices', 'Service notices', false)
		 ON CONFLICT (key) DO UPDATE SET requires_double_opt_in = false
		 RETURNING id`).Scan(&plain); err != nil {
		t.Fatalf("seeding a non-DOI purpose: %v", err)
	}

	rec := postDoubleOptIn(t, e, plain)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("the door answered %d for a purpose needing no confirmation, want 422: %s",
			rec.Code, rec.Body.String())
	}
	var minted int
	if err := e.owner.QueryRow(e.ctx,
		`SELECT count(*) FROM confirm_token WHERE person_id = $1`, e.person).Scan(&minted); err != nil {
		t.Fatalf("counting minted links: %v", err)
	}
	if minted != 0 {
		t.Errorf("%d links were minted for a purpose that needs no confirmation", minted)
	}
}

// marketingPurposeID reads the seeded double-opt-in purpose.
func marketingPurposeID(t *testing.T, e *channelConsentEnv) ids.PurposeID {
	t.Helper()
	var purpose ids.PurposeID
	if err := e.owner.QueryRow(e.ctx,
		`SELECT id FROM consent_purpose WHERE key = $1`, PurposeMarketingEmail).Scan(&purpose); err != nil {
		t.Fatalf("read the marketing purpose: %v", err)
	}
	return purpose
}

// postDoubleOptIn drives the real handler with the environment's authenticated
// context, the way the router does.
func postDoubleOptIn(t *testing.T, e *channelConsentEnv, purpose ids.PurposeID) *httptest.ResponseRecorder {
	t.Helper()
	return postDoubleOptInWith(t, e, Handlers{store: e.store}, purpose)
}

// postDoubleOptInWith drives the door with whatever wiring the caller wants,
// so one arm can watch the mail the lane was handed.
func postDoubleOptInWith(
	t *testing.T, e *channelConsentEnv, h Handlers, purpose ids.PurposeID,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"purpose_id":"` + purpose.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/people/x/consent/double-opt-in", body).WithContext(e.ctx)
	req.Header.Set("Content-Type", "application/json")
	h.IssueDoubleOptIn(rec, req, crmcontracts.Id(e.person.UUID))
	return rec
}
