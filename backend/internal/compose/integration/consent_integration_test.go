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

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type consentEnv struct {
	*apptest.AppEnv
	personID   string
	activityID string
	purposes   map[string]string // key -> id
}

func setupConsent(t *testing.T) *consentEnv {
	t.Helper()
	e := apptest.SetupApp(t)
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
	return &consentEnv{AppEnv: e, personID: person.ID, activityID: activity.ID, purposes: purposes}
}

// consentPurposeWithoutDOI creates a consent-class purpose that confirms by an
// ordinary grant.
//
// The send gate's own claim — no recorded decision suppresses, a recorded grant
// releases — needs A consent-class purpose, not specifically a double-opt-in
// one. It used to ride marketing_email because the operator could mint that
// purpose's confirmation token itself; #3807 removed that endpoint, on the
// ground that one person completing both halves of a round trip is exactly what
// the round trip exists to prevent. Marketing now confirms only through the
// mailed confirm-details link, whose own coverage is
// consent/confirmflow_integration_test.go.
//
// So this lane asks its own question with a purpose it can legitimately grant,
// and TestConsentDoubleOptInNorm below holds the DOI purpose to the refusals
// that are now the whole of its contract surface here.
func (c *consentEnv) consentPurposeWithoutDOI(t *testing.T, key string) string {
	t.Helper()
	var purpose struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/consent-purposes", AnyMap{
		"key": key, "label": key, "requires_double_opt_in": false,
	}, nil, &purpose); status != http.StatusCreated {
		t.Fatalf("create the %s purpose → %d", key, status)
	}
	return purpose.ID
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

	// A recorded grant releases the same gate that suppressed above. Through a
	// consent-class purpose that needs no double opt-in, because the operator
	// can no longer complete a DOI purpose's round trip at all — see
	// consentPurposeWithoutDOI.
	newsletter := c.consentPurposeWithoutDOI(t, "product_newsletter")
	if status, code := c.send(t, "product_newsletter"); status != http.StatusConflict || code != "consent_not_granted" {
		t.Fatalf("send before the grant → %d %q, want 409 consent_not_granted", status, code)
	}
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent", AnyMap{
		"purpose_id": newsletter, "new_state": "granted", "lawful_basis": "consent",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("record consent → %d", status)
	}
	if status, code := c.send(t, "product_newsletter"); status != http.StatusAccepted {
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

// A double-opt-in purpose cannot be granted from this side of the wire at all.
//
// It used to be, and that WAS the defect: the endpoint minted a token and
// returned the plaintext to the authenticated operator, who could paste it
// straight back into the consent write. One person completed both halves of a
// round trip whose entire evidentiary value is that the data subject completed
// it from their own mailbox, and the consent_event recorded a confirmation that
// had not happened (#3807).
//
// So what this lane holds now is the closed door, from every side an operator
// can push on: a bare grant, a forged token, and issuance itself. The grant that
// DOES work arrives through the mailed confirm-details link, whose round trip is
// covered where it lives — consent/confirmflow_integration_test.go.
func TestADoubleOptInPurposeCannotBeGrantedByTheOperator(t *testing.T) {
	c := setupConsent(t)

	// No proof at all.
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent", AnyMap{
		"purpose_id": c.purposes["marketing_email"], "new_state": "granted",
	}, nil, nil); status != 422 {
		t.Fatalf("DOI-less marketing grant → %d, want 422", status)
	}
	// A fabricated token proves nothing — and now there is no un-fabricated one
	// either, which is what makes this arm the whole story rather than half of
	// it.
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent", AnyMap{
		"purpose_id": c.purposes["marketing_email"], "new_state": "granted",
		"double_opt_in_token": "doi-token-forged",
	}, nil, nil); status != 422 {
		t.Fatalf("forged DOI grant → %d, want 422", status)
	}
	// The mint refuses, so nothing an operator holds can ever satisfy the arm
	// above. It answers rather than 404s because the operation is still in the
	// public contract and a caller deserves to be told why.
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent/double-opt-in", AnyMap{
		"purpose_id": c.purposes["marketing_email"],
	}, nil, nil); status != http.StatusConflict {
		t.Fatalf("DOI issuance → %d, want 409 — an operator-held token is exactly what was removed", status)
	}
	// And the send stays suppressed, which is the consequence worth asserting:
	// a closed door that let the message out anyway would be decoration.
	if status, code := c.send(t, "marketing_email"); status != http.StatusConflict || code != "consent_not_granted" {
		t.Fatalf("marketing send after every refused route → %d %q, want 409 consent_not_granted", status, code)
	}
}

// Issuance refuses whatever it is asked about. The purpose-validation and
// supersession this once held are gone with the mint: there is no token to
// validate a purpose for and none to supersede.
//
// Both arms, because "refuses for the RIGHT reason" is the claim. A missing
// purpose id is still a malformed request and is still named as one — an
// endpoint that refuses for its own reasons must not become the one place a
// missing id goes unreported.
func TestDoubleOptInIssuanceRefusesAndMintsNothing(t *testing.T) {
	c := setupConsent(t)

	for _, purpose := range []struct{ name, id string }{
		{"a DOI purpose", c.purposes["marketing_email"]},
		{"a purpose that never required DOI", c.purposes["transactional"]},
	} {
		if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent/double-opt-in", AnyMap{
			"purpose_id": purpose.id,
		}, nil, nil); status != http.StatusConflict {
			t.Errorf("issuance for %s → %d, want 409", purpose.name, status)
		}
	}
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent/double-opt-in",
		AnyMap{}, nil, nil); status != 422 {
		t.Errorf("issuance with no purpose id → %d, want 422 — the missing id is named before the refusal", status)
	}

	// Nothing was minted, and the audit is where that is checkable: the mint
	// used to write one row per issuance, so an endpoint that had quietly kept
	// writing while answering 409 would look identical from the status alone.
	var audit struct {
		Data []AnyMap `json:"data"`
	}
	if status := c.Call(t, "GET", "/v1/audit-log?entity_type=consent_doi_token", nil, nil, &audit); status != http.StatusOK {
		t.Fatalf("audit read → %d", status)
	}
	if len(audit.Data) != 0 {
		t.Errorf("%d double-opt-in issuance(s) audited, want none — the endpoint refuses and mints nothing", len(audit.Data))
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
