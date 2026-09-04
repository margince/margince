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
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type consentEnv struct {
	*apptest.AppEnv
	personID   string
	activityID string
	purposes   map[string]string // key -> id
	// mail is the relay the confirm-details link rides. A double-opt-in purpose
	// can only be granted by spending a mailed link, and the plaintext never
	// appears in a response, so a test has to read the mail.
	mail *capturingMailer
}

func setupConsent(t *testing.T) *consentEnv {
	t.Helper()
	mail := &capturingMailer{}
	e := apptest.SetupAppWithOptions(t, compose.WithOperatorMail(mail))
	apptest.BootstrapWorkspaceSession(t, e, "Consent E2E", "dpo@fable.test", "Admin")

	var person struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/people", AnyMap{
		"full_name": "Consent Subject",
		"emails":    []AnyMap{{"email": "subject@consent.test"}},
	}, nil, &person); status != http.StatusCreated {
		t.Fatalf("create person → %d", status)
	}
	var activity struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "email", "subject": "Inbound question", "direction": "inbound",
		"links": []AnyMap{{"entity_type": "person", "entity_id": person.ID}},
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
	return &consentEnv{AppEnv: e, mail: mail, personID: person.ID, activityID: activity.ID, purposes: purposes}
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
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent", AnyMap{
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

	// The fixture's person wrote to us: setupConsent captures an INBOUND
	// activity from them, which is the qualifying event correspondence needs.
	if status, code := c.send(t, "transactional"); status != http.StatusAccepted {
		t.Fatalf("transactional send → %d %q, want 202 — the contract is the basis, not consent", status, code)
	}
	if status, code := c.send(t, "business_correspondence"); status != http.StatusAccepted {
		t.Fatalf("correspondence send → %d %q, want 202 — they wrote to us first", status, code)
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
		`SELECT count(*) FROM consent_qualifying_event WHERE person_id = $1`, c.personID).Scan(&before); err != nil {
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
		 FROM consent_qualifying_event WHERE person_id = $1`, c.personID).
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
		`SELECT count(*) FROM consent_qualifying_event WHERE person_id = $1`, c.personID).Scan(&after); err != nil {
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
	if status := c.Call(t, "GET", "/v1/people/"+c.personID+"/consent/guard", nil, nil, &guard); status != http.StatusOK {
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
		`SELECT count(*) FROM consent_qualifying_event WHERE person_id = $1`, c.personID).Scan(&recorded); err != nil {
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

	// Detach the address the fixture's person holds. Nothing else changes: the
	// person is still live, and their inbound message still sits on the record.
	if _, err := c.Owner.Exec(ctx,
		`UPDATE person_email SET archived_at = now() WHERE person_id = $1`, c.personID); err != nil {
		t.Fatal(err)
	}

	// The address now belongs to nobody, so it resolves to no person and no
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
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent", AnyMap{
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
	status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent", AnyMap{
		"purpose_id": c.purposes["marketing_email"], "new_state": "granted",
	}, nil, &problem)
	if status != 422 {
		t.Fatalf("DOI-less marketing grant → %d, want 422", status)
	}
	// A fabricated token proves nothing: only a server-issued one confirms.
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent", AnyMap{
		"purpose_id": c.purposes["marketing_email"], "new_state": "granted",
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
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent", AnyMap{
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
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent/confirm-request",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("ask the workspace to mail the confirm link → %d", status)
	}
	token := c.mail.confirmLinkToken(t)
	if s := publicCall(t, c.AppEnv, "POST", "/v1/public/confirm/"+token, AnyMap{
		"marketing_choice":  "granted",
		"marketing_wording": "Yes, send me occasional product news.",
	}, nil, nil); s != http.StatusNoContent {
		t.Fatalf("the subject spends their own link → %d, want 204", s)
	}
	// Returned so a caller can assert what a SPENT link does next.
	return token
}

// Issuance mints nothing, and says so.
//
// This replaces a test of the issuance round trip — non-DOI purposes refused,
// a fresh token superseding a stale one, both mints audited. None of that
// exists now: the endpoint returns a conflict for every caller, nothing writes
// consent_doi_token, and the redemption arm is gone. A double-opt-in purpose
// confirms through a spent confirm-details link instead, which
// TestConsentDoubleOptInNorm exercises.
//
// What is worth holding is that the refusal is a refusal: the same answer
// whatever purpose is named, and no row behind it. An endpoint in the public
// contract that quietly minted again would hand an operator both halves of a
// round trip whose only value is that the subject completed one of them.
func TestDOIIssuanceMintsNothingWhicheverPurposeIsNamed(t *testing.T) {
	c := setupConsent(t)

	// A purpose that requires double opt-in, and one that does not, get the
	// same answer. The endpoint resolves no id, so it can tell a caller nothing
	// about which purposes exist.
	for _, purpose := range []string{"marketing_email", "transactional"} {
		if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent/double-opt-in", AnyMap{
			"purpose_id": c.purposes[purpose],
		}, nil, nil); status != http.StatusConflict {
			t.Errorf("issuance under %s → %d, want 409", purpose, status)
		}
	}

	// And nothing was written. The table keeps its history and takes no new
	// rows, so an unredeemed invitation cannot outlive the change.
	var audit struct {
		Data []AnyMap `json:"data"`
	}
	if status := c.Call(t, "GET", "/v1/audit-log?entity_type=consent_doi_token", nil, nil, &audit); status != http.StatusOK {
		t.Fatalf("audit read → %d", status)
	}
	if len(audit.Data) != 0 {
		t.Fatalf("a refused issuance audited %d row(s), want none", len(audit.Data))
	}
}

func TestConsentProofLogIsAppendOnlyAndIdempotent(t *testing.T) {
	c := setupConsent(t)
	grant := func() int {
		return c.Call(t, "POST", "/v1/people/"+c.personID+"/consent", AnyMap{
			"purpose_id": c.purposes["transactional"], "new_state": "granted",
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
	if status := c.Call(t, "GET", "/v1/people/"+c.personID+"/consent", nil, nil, &state); status != http.StatusOK {
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
// was asked at all. The person here has nothing on file, so the answer is a
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
		"links": []AnyMap{{"entity_type": "person", "entity_id": c.personID}},
	}, nil, &problem)
	if status != http.StatusConflict {
		t.Fatalf("an unevidenced account send with no consent_purpose → %d %q, want 409 — omitting the claim is not evidence",
			status, problem.Code)
	}
}
