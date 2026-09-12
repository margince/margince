// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Consent enforcement end to end (B-EP07.11/.12, A22/ADR-0011): the
// purpose catalog seeds at bootstrap, recordConsent writes the
// append-only proof + audit + event, and the send path is default-deny
// per purpose — unknown blocks, a foreign-purpose grant blocks,
// withdrawal re-blocks, and the German double-opt-in norm holds.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type consentEnv struct {
	*apptest.AppEnv
	contactID  string
	activityID string
	dealID     string
	purposes   map[string]string // key -> id
}

func setupConsent(t *testing.T) *consentEnv {
	t.Helper()
	// The relay is wired because a controller delivery resolves its transport
	// before any gate runs; the LINK is read from the vault, where minting
	// sealed it (confirmlinkmail_integration_test.go).
	e := apptest.SetupAppWithOptions(t, compose.WithOperatorMail(discardingMailer{}))
	apptest.BootstrapWorkspaceSession(t, e, "Consent E2E", "dpo@fable.test", "Admin")

	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
		"full_name": "Consent Subject",
		"emails":    []AnyMap{{"email": "subject@consent.test"}},
	}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact → %d", status)
	}
	var activity struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "email", "subject": "Inbound question", "direction": "inbound",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": contact.ID}},
	}, nil, &activity); status != http.StatusCreated {
		t.Fatalf("log anchor activity → %d", status)
	}

	var purposeList struct {
		Data []struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/consent-purposes", nil, nil, &purposeList); status != http.StatusOK {
		t.Fatalf("list purposes → %d", status)
	}
	purposes := map[string]string{}
	for _, p := range purposeList.Data {
		purposes[p.Key] = p.ID
	}
	if purposes["transactional"] == "" || purposes["marketing_email"] == "" ||
		purposes["business_correspondence"] == "" {
		t.Fatalf("bootstrap did not seed the purpose catalog: %+v", purposeList.Data)
	}
	return &consentEnv{AppEnv: e, contactID: contact.ID, activityID: activity.ID, purposes: purposes}
}

// stakeADeal gives the fixture's subject real transactional evidence: an
// open deal with them staked on it, relinked onto c.activityID (a reply
// inherits its links from the anchor). Opt in for the same reason
// preflightEnv.stakeADeal is: resolveCategory's live-deal arm runs before
// any purpose is asked about, so baking this into setupConsent would
// silently change the basis every OTHER test in this file — most of which
// are specifically about a purpose with no evidence behind it — is denied
// on.
func (c *consentEnv) stakeADeal(t *testing.T) {
	t.Helper()
	stages := apptest.DiscoverSeededPipeline(t, c.AppEnv)
	c.dealID = apptest.StakeOnOpenDeal(t, c.AppEnv, "Consent E2E opportunity", stages, c.contactID)
	if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/relink", AnyMap{
		"entity_type": "deal", "entity_id": c.dealID,
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("relink the anchor onto the deal → %d", status)
	}
}

func (c *consentEnv) send(t *testing.T, purpose string) (int, string) {
	t.Helper()
	var problem struct {
		Code string `json:"code"`
	}
	status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
		"subject": "Re: Inbound question", "body": "answer",
		"to": []string{"subject@consent.test"}, "consent_purpose": purpose,
	}, nil, &problem)
	return status, problem.Code
}

func TestConsentDefaultDenySuppressesSends(t *testing.T) {
	c := setupConsent(t)

	// Drafting is 🟢 and consent-free — it sends nothing.
	var draft struct {
		Subject string `json:"subject"`
	}
	if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/draft-email",
		AnyMap{"intent": "friendly nudge"}, nil, &draft); status != http.StatusOK {
		t.Fatalf("draft → %d", status)
	}
	if draft.Subject != "Re: Inbound question" {
		t.Fatalf("draft subject = %q", draft.Subject)
	}

	// A consent-CLASS purpose with no recorded decision is suppressed. The
	// purpose under test is marketing rather than transactional: ADR-0098
	// classes transactional and business correspondence as never
	// consent-gated, so neither can carry the default-deny claim any more.
	if status, code := c.send(t, "marketing_email"); status != http.StatusConflict || code != "consent_not_granted" {
		t.Fatalf("send with unknown consent → %d %q, want 409 consent_not_granted", status, code)
	}
	// An undefined purpose can authorize nothing.
	if status, code := c.send(t, "no-such-purpose"); status != http.StatusConflict || code != "consent_not_granted" {
		t.Fatalf("send under unknown purpose → %d %q", status, code)
	}

	// Grant marketing through the round-trip its purpose demands; the send
	// under THAT purpose then flows.
	// The grant IS the spent link: the public edge records it through the
	// ordinary consent engine, so there is no second call to make here.
	c.grantMarketingByConfirmLink(t)
	if status, code := c.send(t, "marketing_email"); status != http.StatusAccepted {
		t.Fatalf("granted send → %d %q, want 202", status, code)
	}

	// Withdrawal re-blocks, and it does so through the objection rule that
	// overrides every other basis.
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent", AnyMap{
		"purpose_id": c.purposes["marketing_email"], "new_state": "withdrawn",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("withdraw → %d", status)
	}
	if status, code := c.send(t, "marketing_email"); status != http.StatusConflict || code != "consent_not_granted" {
		t.Fatalf("post-withdrawal send → %d %q, want 409", status, code)
	}
}

// Answering somebody is not advertising to them (ADR-0098 D1/D2).
//
// This is the rule that ADR-0011's blanket default-deny got wrong: under it a
// rep answering an inbound question was formally a consent violation until
// somebody recorded a grant, which is legally wrong and which every rep
// correctly ignored. Correspondence is allowed on a recorded qualifying event
// and needs no consent object at all — while transactional mail, whose basis is
// the contract itself, needs neither.
func TestCorrespondenceAndTransactionalAreNotConsentGated(t *testing.T) {
	c := setupConsent(t)
	// A live deal — real evidence, now that a bare purpose claim no longer
	// carries itself. setupConsent's own anchor is hand-logged rather than
	// captured, so it carries no thread_key for the reply arm to answer
	// through (only real capture threads a conversation); the deal is what
	// stands in for "they have a live reason to hear from us" here.
	c.stakeADeal(t)

	if status, code := c.send(t, "transactional"); status != http.StatusAccepted {
		t.Fatalf("transactional send → %d %q, want 202 — the contract is the basis, not consent", status, code)
	}
	if status, code := c.send(t, "business_correspondence"); status != http.StatusAccepted {
		t.Fatalf("correspondence send → %d %q, want 202 — the live deal is the basis, not consent", status, code)
	}
}

// A basis the gate DERIVED is stamped onto the record before it is relied on
// (ADR-0098 D2, Art 5(2)).
//
// Deriving the qualifying event from the timeline answers the question
// correctly but not accountably: nothing on the record would say what
// authorized this particular send. The controller carries the burden of
// showing a lawful basis, and a computation this build happened to make is not
// something anybody can look up afterwards.
func TestASendOnADerivedBasisRecordsWhatAuthorizedIt(t *testing.T) {
	c := setupConsent(t)
	ctx := context.Background()

	var before int
	if err := c.Owner.QueryRow(ctx,
		`SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1`, c.contactID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 0 {
		t.Fatalf("the fixture starts with %d qualifying events, want none — the derivation is what allows the send", before)
	}

	if status, code := c.send(t, "business_correspondence"); status != http.StatusAccepted {
		t.Fatalf("correspondence send → %d %q, want 202", status, code)
	}

	var kind, sourceType, source string
	if err := c.Owner.QueryRow(ctx,
		`SELECT kind, source_entity_type, source
		 FROM consent_qualifying_event WHERE contact_id = $1`, c.contactID).
		Scan(&kind, &sourceType, &source); err != nil {
		t.Fatalf("the send was allowed on a derived basis that was never recorded: %v", err)
	}
	if kind != "inbound_message" || sourceType != "activity" {
		t.Errorf("recorded %s/%s, want inbound_message/activity — the stamp must name the message that allowed it", kind, sourceType)
	}
	if source != "derived" {
		t.Errorf("recorded source %q, want %q — a derived basis must not read as one a human typed", source, "derived")
	}

	// A second send re-derives the same message. It must not stack a second
	// row claiming a second event happened.
	if status, _ := c.send(t, "business_correspondence"); status != http.StatusAccepted {
		t.Fatal("the second correspondence send should still be allowed")
	}
	var after int
	if err := c.Owner.QueryRow(ctx,
		`SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1`, c.contactID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != 1 {
		t.Errorf("qualifying events = %d after two sends, want 1 — the same message is one event", after)
	}
}

// A preview authorizes nothing, so it records nothing.
//
// Writing a legal fact because somebody opened a composer would put a lawful
// basis on the record for a message that was never sent.
func TestTheGuardPreviewRecordsNothing(t *testing.T) {
	c := setupConsent(t)
	ctx := context.Background()

	var guard struct {
		Entries []struct {
			PurposeKey string `json:"purpose_key"`
			Verdict    string `json:"verdict"`
		} `json:"entries"`
	}
	if status := c.Call(t, "GET", "/v1/contacts/"+c.contactID+"/consent/guard", nil, nil, &guard); status != http.StatusOK {
		t.Fatalf("guard → %d", status)
	}
	var sawCorrespondence bool
	for _, entry := range guard.Entries {
		if entry.PurposeKey == "business_correspondence" {
			sawCorrespondence = true
			if entry.Verdict != "allowed" {
				t.Errorf("correspondence guard = %q, want allowed — they wrote to us", entry.Verdict)
			}
		}
	}
	if !sawCorrespondence {
		t.Fatal("the guard did not report on business correspondence at all")
	}

	var recorded int
	if err := c.Owner.QueryRow(ctx,
		`SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1`, c.contactID).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 0 {
		t.Errorf("the preview wrote %d qualifying event(s); a preview authorizes nothing and must record nothing", recorded)
	}
}

// An archived address is one somebody detached from a record, and the same
// string may be live on somebody else. Resolving through it would answer about
// the wrong human.
func TestAnArchivedAddressDoesNotAuthorizeItsFormerHolder(t *testing.T) {
	c := setupConsent(t)
	ctx := context.Background()

	// Detach the address the fixture's contact holds. Nothing else changes: the
	// contact is still live, and their inbound message still sits on the record.
	if _, err := c.Owner.Exec(ctx,
		`UPDATE contact_email SET archived_at = now() WHERE contact_id = $1`, c.contactID); err != nil {
		t.Fatal(err)
	}

	// The address now belongs to nobody, so it resolves to no contact and no
	// lead — and default-deny refuses rather than reaching the former holder's
	// qualifying event.
	if status, code := c.send(t, "business_correspondence"); status != http.StatusConflict || code != "consent_not_granted" {
		t.Fatalf("send to a detached address → %d %q, want 409 — an archived identity authorizes nobody", status, code)
	}
}

// An objection is absolute: Art 21(2)-(3) admits no balancing, so a withdrawal
// on the correspondence purpose outranks the qualifying event that would
// otherwise allow it. There is no override toggle, and there must be no path
// through the class model that reaches past a suppression.
func TestAnObjectionOverridesAQualifyingEvent(t *testing.T) {
	c := setupConsent(t)

	if status, _ := c.send(t, "business_correspondence"); status != http.StatusAccepted {
		t.Fatal("the fixture's inbound message should allow correspondence before the objection")
	}
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent", AnyMap{
		"purpose_id": c.purposes["business_correspondence"], "new_state": "withdrawn",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("record the objection → %d", status)
	}
	if status, code := c.send(t, "business_correspondence"); status != http.StatusConflict || code != "consent_not_granted" {
		t.Fatalf("post-objection correspondence → %d %q, want 409 — an objection overrides the qualifying event", status, code)
	}
}

// The consent gate must never be an oracle: a caller who cannot see
// the anchor gets the anchor's own refusal (404), not a consent answer.
func TestConsentGateIsNotAnOracleForUnauthorizedCallers(t *testing.T) {
	c := setupConsent(t)
	var problem struct {
		Code string `json:"code"`
	}
	status := c.Call(t, "POST", "/v1/activities/00000000-0000-7000-8000-000000000001/send-email", AnyMap{
		"subject": "probe", "body": "probe",
		"to": []string{"subject@consent.test"}, "consent_purpose": "transactional",
	}, nil, &problem)
	if status != http.StatusNotFound {
		t.Fatalf("send against an invisible anchor → %d %q, want 404 before any consent signal", status, problem.Code)
	}
}

func TestConsentDoubleOptInNorm(t *testing.T) {
	c := setupConsent(t)

	// marketing_email requires DOI: a bare grant is refused outright.
	var problem struct {
		Code string `json:"code"`
	}
	status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent", AnyMap{
		"purpose_id": c.purposes["marketing_email"], "new_state": "granted",
		"wording": "Yes, you may contact me about this.",
	}, nil, &problem)
	if status != 422 {
		t.Fatalf("DOI-less marketing grant → %d, want 422", status)
	}
	// A fabricated token proves nothing: only a server-issued one confirms.
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent", AnyMap{
		"purpose_id": c.purposes["marketing_email"], "new_state": "granted",
		"wording":             "Yes, you may contact me about this.",
		"double_opt_in_token": "doi-token-forged",
	}, nil, nil); status != 422 {
		t.Fatalf("forged DOI grant → %d, want 422", status)
	}

	// The real round trip: the workspace mails the link, the subject spends it
	// from their own mailbox, and the send under that purpose then flows.
	spent := c.grantMarketingByConfirmLink(t)
	if status, code := c.send(t, "marketing_email"); status != http.StatusAccepted {
		t.Fatalf("DOI-granted send → %d %q, want 202", status, code)
	}

	// The token is single-use: after a withdrawal the consumed token
	// cannot resurrect the grant.
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent", AnyMap{
		"purpose_id": c.purposes["marketing_email"], "new_state": "withdrawn",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("withdraw → %d", status)
	}
	// And the spent link cannot put it back. A withdrawal that a replayed mail
	// could undo would make the withdrawal advisory — the subject has to be
	// asked again, with a link they have not already used.
	if s := publicCall(t, c.AppEnv, "POST", "/v1/public/confirm/"+spent, AnyMap{
		"marketing_choice":  "granted",
		"marketing_wording": "Yes, send me occasional product news.",
	}, nil, nil); s != http.StatusNotFound {
		t.Fatalf("replaying the spent link → %d, want 404", s)
	}
}

// grantMarketingByConfirmLink takes the subject through the only round trip that
// can now grant a double-opt-in purpose: the workspace mails them a link, and
// they spend it with their answer.
//
// Both halves are real HTTP, and they are deliberately different callers. The
// operator asks for the mail; the SUBJECT posts the answer through the anonymous
// public edge carrying nothing but the token that reached their mailbox. That
// separation is the whole evidentiary claim, and it is why there is no operator
// shortcut to shorten this helper with.
func (c *consentEnv) grantMarketingByConfirmLink(t *testing.T) string {
	t.Helper()
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/confirm-request",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("ask the workspace to mail the confirm link → %d", status)
	}
	token := confirmLinkToken(t, c.AppEnv)
	// The submission answers with a receipt now. A marketing answer alone
	// proposes no correction and asks for no erasure, so it opens no rights
	// case — and an empty list is the assertion that says so: a helper every
	// consent test leans on is where a marketing tick quietly filing Art. 16
	// cases into the admin queue would first be visible.
	var receipt crmcontracts.ConfirmSubmissionReceipt
	if s := publicCall(t, c.AppEnv, "POST", "/v1/public/confirm/"+token, AnyMap{
		"marketing_choice":  "granted",
		"marketing_wording": "Yes, send me occasional product news.",
	}, nil, &receipt); s != http.StatusOK {
		t.Fatalf("the subject spends their own link → %d, want 200", s)
	}
	if len(receipt.Cases) != 0 {
		t.Fatalf("the marketing answer opened %d rights case(s), want none", len(receipt.Cases))
	}
	// Returned so a caller can assert what a SPENT link does next.
	return token
}

func TestConsentProofLogIsAppendOnlyAndIdempotent(t *testing.T) {
	c := setupConsent(t)
	grant := func() int {
		return c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent", AnyMap{
			"purpose_id": c.purposes["transactional"], "new_state": "granted",
			"wording": "Yes, you may contact me about this.",
		}, nil, nil)
	}
	if status := grant(); status != http.StatusOK {
		t.Fatalf("grant → %d", status)
	}
	// Re-asserting the same state is idempotent: no second proof row.
	if status := grant(); status != http.StatusOK {
		t.Fatalf("re-grant → %d", status)
	}
	var state struct {
		State []struct {
			PurposeKey string `json:"purpose_key"`
			State      string `json:"state"`
		} `json:"state"`
		Events []struct {
			NewState string `json:"new_state"`
		} `json:"events"`
	}
	if status := c.Call(t, "GET", "/v1/contacts/"+c.contactID+"/consent", nil, nil, &state); status != http.StatusOK {
		t.Fatalf("get consent → %d", status)
	}
	if len(state.Events) != 1 {
		t.Fatalf("idempotent re-grant appended a proof row: %d events", len(state.Events))
	}
	// Every tracked purpose reads back — absent ones as honest unknown.
	byKey := map[string]string{}
	for _, st := range state.State {
		byKey[st.PurposeKey] = st.State
	}
	if byKey["transactional"] != "granted" || byKey["marketing_email"] != "unknown" {
		t.Fatalf("state readback wrong: %+v", byKey)
	}
	// The consent change is audited and on the bus.
	var audits, events int
	if err := c.Owner.QueryRow(t.Context(),
		`SELECT count(*) FROM audit_log WHERE action = 'consent_grant'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := c.Owner.QueryRow(t.Context(),
		fmt.Sprintf(`SELECT count(*) FROM event_outbox WHERE envelope->>'type' = '%s'`, "consent.changed")).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if audits != 1 || events != 1 {
		t.Fatalf("audit/event counts = %d/%d, want 1/1", audits, events)
	}
}

// TestASendWithNoPurposeKeyReachesTheEngine is what makes the legacy purposes
// retireable.
//
// consent_purpose was a required field for as long as it was the authority.
// The engine decides now, from the record — so a caller that omits the key is
// not withholding an answer, it is declining to make a claim the engine was
// going to check against the tables anyway.
//
// The assertion is that the request is JUDGED rather than rejected as
// malformed: a 409 naming a consent code is the engine answering, and it is a
// different outcome from the 422 the contract used to produce before consent
// was asked at all. The contact here has nothing on file, so the answer is a
// refusal — which is the correct one, and the point is who gave it.
//
// This is the case that has to work before `transactional` and
// `business_correspondence` are archived. While the key was required,
// archiving them left every caller with nothing valid to name.
//
// Mutation: put consent_purpose back on the schema's `required` list and this
// fails with 422 validation_error.
func TestASendWithNoPurposeKeyReachesTheEngine(t *testing.T) {
	c := setupConsent(t)

	var problem struct {
		Code string `json:"code"`
	}
	status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
		"subject": "Re: Inbound question", "body": "answer",
		"to": []string{"subject@consent.test"},
	}, nil, &problem)
	if status == http.StatusUnprocessableEntity {
		t.Fatalf("a send with no consent_purpose → 422 %q; the contract still demands a key the engine no longer needs",
			problem.Code)
	}
	if status != http.StatusConflict || problem.Code != "consent_not_granted" {
		t.Fatalf("a send with no consent_purpose → %d %q, want 409 consent_not_granted — the engine judged it on the record",
			status, problem.Code)
	}
}

// TestOmittingThePurposeKeyIsNotAWayPastTheGate holds the direction the
// relaxation must not break.
//
// Making the key optional must not turn omitting it into an allow. A message
// with no thread, no deal and no evidence has nothing supporting it, and
// dropping the claim does not supply one.
//
// It differs from the test above in the recipient: that one asks whether the
// engine ANSWERED, this one asks whether a stranger is still refused. Both
// currently refuse, and they would diverge the moment an empty claim started
// resolving to something supported — which is the regression this pins.
//
// Mutation: make resolveFromClaimAndPurpose treat an empty key as supported
// and this fails with a 202.
func TestOmittingThePurposeKeyIsNotAWayPastTheGate(t *testing.T) {
	c := setupConsent(t)

	var problem struct {
		Code string `json:"code"`
	}
	status := c.Call(t, "POST", "/v1/emails", AnyMap{
		"subject": "Something unrelated", "body": "out of the blue",
		"to":    []string{"subject@consent.test"},
		"links": []AnyMap{{"entity_type": "contact", "entity_id": c.contactID}},
	}, nil, &problem)
	if status != http.StatusConflict {
		t.Fatalf("an unevidenced account send with no consent_purpose → %d %q, want 409 — omitting the claim is not evidence",
			status, problem.Code)
	}
}

// TestThePreviewAgreesWithTheSendItPreviews is the whole point of the endpoint.
//
// A preview that could differ from the send would be worse than no preview: a
// rep told "this will go" and then refused has been misled at the moment they
// were trying to be careful. Both run the same decideOne over the same request,
// and this holds that by asking BOTH and comparing.
//
// Mutation: point the preview at a different resolver and the two answers
// diverge on the second case.
func TestThePreviewAgreesWithTheSendItPreviews(t *testing.T) {
	c := setupConsent(t)

	// The fixture's contact has an inbound on file, so correspondence is
	// supported and marketing is not. Two cases with opposite answers, because
	// a preview that always said "allowed" would pass a one-case test.
	for _, tc := range []struct {
		name    string
		purpose string
		want    bool
	}{
		{"correspondence, which the timeline supports", "business_correspondence", true},
		{"marketing, which needs a grant nobody recorded", "marketing_email", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var preview struct {
				Allowed    bool `json:"allowed"`
				Recipients []struct {
					Verdict    string `json:"verdict"`
					ReasonCode string `json:"reason_code"`
				} `json:"recipients"`
			}
			// The SAME inputs the send is about to make, purpose key included.
			// Comparing a preview asked about one message with a send that made
			// another would prove nothing about drift.
			if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email:preview", AnyMap{
				"to": []string{"subject@consent.test"}, "consent_purpose": tc.purpose,
			}, nil, &preview); status != http.StatusOK {
				t.Fatalf("preview → %d, want 200", status)
			}
			if len(preview.Recipients) != 1 {
				t.Fatalf("preview answered about %d recipients, want 1", len(preview.Recipients))
			}

			status, code := c.send(t, tc.purpose)
			sent := status == http.StatusAccepted

			if preview.Allowed != sent {
				t.Errorf("preview said allowed=%v and the send %s (%d %q) — the two must not disagree about an unchanged record",
					preview.Allowed, map[bool]string{true: "went", false: "was refused"}[sent], status, code)
			}
			if sent != tc.want {
				t.Errorf("the send %s, want %v — the fixture no longer sets up the case this row names",
					map[bool]string{true: "went", false: "was refused"}[sent], tc.want)
			}
		})
	}
}

// TestThePreviewRefusesARecordTheCallerCannotRead holds the order of the two
// checks, which is a disclosure rule rather than a nicety.
//
// The origin resolves BEFORE anything is asked about consent. Without that, a
// caller could name a stranger's deal and have the engine answer about it — an
// unauthorized read wearing a preview, and an existence oracle for records they
// may not open.
//
// Mutation: move origin.resolve after the previewer call in PreviewSend and
// this returns 200 with an answer about a record the caller cannot see.
func TestThePreviewRefusesARecordTheCallerCannotRead(t *testing.T) {
	c := setupConsent(t)

	var problem struct {
		Code string `json:"code"`
	}
	status := c.Call(t, "POST", "/v1/emails:preview", AnyMap{
		"to":    []string{"subject@consent.test"},
		"links": []AnyMap{{"entity_type": "deal", "entity_id": "00000000-0000-4000-8000-000000000001"}},
	}, nil, &problem)
	if status != http.StatusNotFound {
		t.Errorf("preview naming an unreadable deal → %d %q, want 404 — a record the caller cannot open must answer with the row-scope verdict, not with somebody's consent state",
			status, problem.Code)
	}
}

// TestThePreviewRecordsNothing is the contract the endpoint's description
// makes, checked rather than promised.
//
// decideOne reaches stampDerivedBasis on an allow, which WRITES. Gate.Preview
// rolls its transaction back, and this is what makes that true of the code: a
// composer that previewed ten drafts and sent none would otherwise leave ten
// authorizations for messages nobody sent, and a subject-access export would
// show a basis this installation relied on to send nothing.
//
// Mutation: return nil instead of errPreviewComplete from Preview's
// transaction, so it commits, and the qualifying-event count goes up.
func TestThePreviewRecordsNothing(t *testing.T) {
	c := setupConsent(t)
	ctx := context.Background()

	// BOTH tables a decision can write, because which one it reaches depends on
	// whether the record supports the category on its own: a supported
	// resolution writes communication_basis through recordBasis, and one that
	// falls through to the legacy verdict writes consent_qualifying_event
	// through stampDerivedBasis. Counting only the first passed against a
	// committing Preview on this fixture, which is exactly the mutation this
	// test exists to catch.
	var beforeBasis, beforeDecisions int
	if err := c.Owner.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM communication_basis)
		     + (SELECT count(*) FROM consent_qualifying_event)`).Scan(&beforeBasis); err != nil {
		t.Fatal(err)
	}
	if err := c.Owner.QueryRow(ctx, `SELECT count(*) FROM communication_decision`).Scan(&beforeDecisions); err != nil {
		t.Fatal(err)
	}

	var preview struct {
		Allowed bool `json:"allowed"`
	}
	// A preview the engine ALLOWS, because an allow is the only path that
	// reaches a basis write — a refusal records nothing whatever the
	// transaction does, so a denied preview would pass this test against a
	// Preview that commits.
	if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email:preview", AnyMap{
		"to": []string{"subject@consent.test"}, "consent_purpose": "business_correspondence",
	}, nil, &preview); status != http.StatusOK {
		t.Fatalf("preview → %d, want 200", status)
	}
	if !preview.Allowed {
		t.Fatal("the preview was refused, so it would record nothing whatever the transaction did — this test needs the allow path")
	}

	var afterBasis, afterDecisions int
	if err := c.Owner.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM communication_basis)
		     + (SELECT count(*) FROM consent_qualifying_event)`).Scan(&afterBasis); err != nil {
		t.Fatal(err)
	}
	if err := c.Owner.QueryRow(ctx, `SELECT count(*) FROM communication_decision`).Scan(&afterDecisions); err != nil {
		t.Fatal(err)
	}
	if afterBasis != beforeBasis {
		t.Errorf("the preview recorded %d lawful-basis row(s) — a basis is the ground a SEND relies on, and nothing was sent", afterBasis-beforeBasis)
	}
	if afterDecisions != beforeDecisions {
		t.Errorf("the preview wrote %d decision row(s) — a preview authorizes nothing", afterDecisions-beforeDecisions)
	}
}

// TestADeadAddressRefusesTheNextSend is what the bounce stop is FOR.
//
// Writing the suppression is only half the control: the half that matters is
// that the send path then refuses. Before the writer existed, `hard_bounce` sat
// in the table's kind CHECK and in the engine's reason map with nothing to
// produce one, so this refusal was reachable in the code and unreachable in
// production — the shape that reads as though the product handles dead
// addresses while it sends to them.
//
// Business correspondence is the purpose deliberately, not marketing. ADR-0098
// classes correspondence as never consent-gated, so it is the case that would
// go out if the stop did not bind — and hard_bounce is meant to bind EVERY
// category, because no template makes a dead mailbox accept mail.
//
// EACH SEND GETS ITS OWN INBOUND. The first version of this reused the
// fixture's single anchor and passed with the stop pointing at a completely
// different address — the second send was refused because one inbound supports
// one reply, not because of anything this slice built. A test that cannot tell
// its own subject from an unrelated rule proves nothing about either.
func TestADeadAddressRefusesTheNextSend(t *testing.T) {
	c := setupConsent(t)

	// Correspondence goes, which is what makes the refusal below meaningful:
	// without this the test could pass against a fixture refusing everything.
	if status, code, _ := c.sendFrom(t, c.inbound(t), "business_correspondence"); status != http.StatusAccepted {
		t.Fatalf("correspondence to a live address → %d %q, want 202", status, code)
	}

	// A SECOND inbound, so the send below is refused only by the stop. Proved
	// by the control case: with no stop written, this send is accepted.
	fresh := c.inbound(t)
	if status, code, _ := c.sendFrom(t, fresh, "business_correspondence"); status != http.StatusAccepted {
		t.Fatalf("a second inbound supports its own reply → %d %q, want 202 — if this "+
			"refuses, the test below cannot tell the stop from the anchor rule", status, code)
	}

	// The address dies. Written through the real writer under the CONNECTOR
	// principal that carries a delivery report in production, so the stop is
	// captured by the same actor the observer runs as.
	wsID := apptest.InstallationWorkspaceUUID(context.Background(), t, c.Pool)
	reporter := principal.WithWorkspaceID(context.Background(), wsID)
	reporter = principal.WithCorrelationID(reporter, ids.NewV7())
	reporter = principal.WithActor(reporter, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail",
	})
	if err := database.WithWorkspaceTx(reporter, c.Pool, func(tx pgx.Tx) error {
		return consent.RecordHardBounceTx(reporter, tx, consent.HardBounceFact{
			Address: "subject@consent.test", DeliveryID: ids.NewV7(),
		})
	}); err != nil {
		t.Fatalf("stopping the dead address: %v", err)
	}

	status, code, detail := c.sendFrom(t, c.inbound(t), "business_correspondence")
	if status == http.StatusAccepted {
		t.Fatal("the send went to an address that refused delivery permanently: the stop is " +
			"written and the engine reads it, so a message going out here means the two are " +
			"not connected — which is the exact state before this writer existed")
	}
	if status != http.StatusConflict || code != "consent_not_granted" {
		t.Errorf("send to a dead address → %d %q, want 409 consent_not_granted", status, code)
	}
	// THE REASON, not only the refusal, and this is the assertion that carries
	// the test. Several rules refuse a correspondence send under the same
	// problem code, so a status check alone cannot tell this slice's stop from
	// an unrelated one — an earlier version of this test passed with the stop
	// written against a completely different address, because something else
	// was refusing and nothing checked what.
	if !strings.Contains(detail, "hard_bounce") {
		t.Errorf("the refusal reads %q and does not name hard_bounce: the send was stopped "+
			"by some other rule, so this says nothing about whether a dead address stops mail",
			detail)
	}
}

// inbound logs one fresh inbound message from the subject and answers its id.
// A reply is anchored to the message it answers, so a test making several sends
// needs several anchors.
func (c *consentEnv) inbound(t *testing.T) string {
	t.Helper()
	var activity struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "email", "subject": "Inbound question", "direction": "inbound",
		"links": []AnyMap{{"entity_type": "contact", "entity_id": c.contactID}},
	}, nil, &activity); status != http.StatusCreated {
		t.Fatalf("log an inbound → %d", status)
	}
	return activity.ID
}

// sendFrom is c.send with the anchor named and the DETAIL returned, for a test
// that has to tell one refusal from another.
//
// The detail matters because several rules refuse a correspondence send and the
// problem code is the same for all of them. A staging refusal names the reason
// code of the first denied recipient in its message, which is the only place
// the engine's actual reason reaches a caller — the per-recipient decision rows
// are written at TRANSMIT, and a send refused at staging never gets that far.
func (c *consentEnv) sendFrom(t *testing.T, activityID, purpose string) (int, string, string) {
	t.Helper()
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	status := c.Call(t, "POST", "/v1/activities/"+activityID+"/send-email", AnyMap{
		"subject": "Re: Inbound question", "body": "answer",
		"to": []string{"subject@consent.test"}, "consent_purpose": purpose,
	}, nil, &problem)
	return status, problem.Code, problem.Detail
}
