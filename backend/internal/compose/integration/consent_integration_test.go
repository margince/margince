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

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
	c.grantMarketingThroughTheConfirmLink(t)
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
	// AND NO OPERATOR CAN CLOSE IT FOR THEM. The endpoint that once handed the
	// caller a plaintext token refuses now, because holding it meant the round
	// trip could complete without the subject's mailbox ever taking part.
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent/double-opt-in", AnyMap{
		"purpose_id": c.purposes["marketing_email"],
	}, nil, nil); status != http.StatusConflict {
		t.Fatalf("operator-held DOI issuance → %d, want 409 — the token must reach the subject, not the caller", status)
	}

	// The round trip as it works now: a single-use link to the subject's own
	// address, answered on the anonymous page it opens.
	c.grantMarketingThroughTheConfirmLink(t)
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
	// A bare re-grant is refused for the same reason the first one was: the
	// purpose requires the subject's own confirmation, and a withdrawal does
	// not leave one lying around to reuse.
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent", AnyMap{
		"purpose_id": c.purposes["marketing_email"], "new_state": "granted",
	}, nil, nil); status != 422 {
		t.Fatalf("re-grant with no fresh confirmation → %d, want 422", status)
	}
}

// grantMarketingThroughTheConfirmLink takes the subject through the round trip
// marketing_email requires, as it works now: a single-use link is minted for
// their own live address, and the answer arrives on the anonymous page that
// link opens.
//
// The token is minted through the STORE rather than the contract, because the
// contract deliberately never returns it — `confirm-request` mails it and hands
// the caller nothing. That is the security property the old double-opt-in
// issuance lacked: an operator who holds the plaintext can close the round trip
// without the subject's mailbox taking part, which is why that endpoint now
// refuses. A test needs the seam the mailer would read.
func (c *consentEnv) grantMarketingThroughTheConfirmLink(t *testing.T) {
	t.Helper()
	person, err := ids.ParseAs[ids.PersonKind](c.personID)
	if err != nil {
		t.Fatalf("parsing the person id: %v", err)
	}
	issued, err := consent.NewStore(c.DB()).IssueConfirmToken(c.confirmMinterContext(t), person)
	if err != nil {
		t.Fatalf("mint the confirm link: %v", err)
	}
	if status := c.Call(t, "POST", "/v1/public/confirm/"+issued.Token, AnyMap{
		"marketing_choice":  "granted",
		"marketing_wording": "Yes, send me product news.",
	}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("answer the confirm page → %d, want 204", status)
	}
}

// OPERATOR-HELD ISSUANCE IS RETIRED, and the endpoint says so rather than
// disappearing.
//
// It once minted a plaintext token and returned it to the authenticated caller.
// A double opt-in is evidence only because the data subject completed it from
// their own mailbox, and a caller holding the token could close that round trip
// with the mailbox never taking part — recording a confirmation that had not
// happened. So it refuses for every purpose now, DOI-requiring or not, and the
// answer names why. The replacement is the confirm-details link, whose token is
// mailed to the subject's own live address and returned to nobody.
//
// It refuses rather than 404ing because the operation is in the public
// contract: an integrator who calls it deserves the reason, not the suggestion
// that they got the path wrong.
func TestOperatorHeldDoubleOptInIssuanceRefuses(t *testing.T) {
	c := setupConsent(t)

	for name, purpose := range map[string]string{
		"a purpose that requires double opt-in": c.purposes["marketing_email"],
		"one that does not":                     c.purposes["transactional"],
	} {
		t.Run(name, func(t *testing.T) {
			var problem struct {
				Detail string `json:"detail"`
			}
			if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent/double-opt-in", AnyMap{
				"purpose_id": purpose,
			}, nil, &problem); status != http.StatusConflict {
				t.Fatalf("issuance → %d, want 409", status)
			}
			if !strings.Contains(problem.Detail, "data subject") {
				t.Errorf("the refusal reads %q, want it to say the subject must complete it", problem.Detail)
			}
		})
	}

	// A malformed request is still malformed. The handler probes the required
	// id BEFORE refusing for its own reasons, so an endpoint that says no does
	// not become the one place a missing purpose goes unnamed.
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent/double-opt-in",
		AnyMap{}, nil, nil); status != 422 {
		t.Fatalf("issuance with no purpose → %d, want 422", status)
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

// confirmMinterContext is the seat the MAIL PATH would mint under — a human who
// may update the person the link is for, which is what IssueConfirmToken
// requires. Built here rather than borrowed from a request because the contract
// route deliberately hands the token to nobody.
func (c *consentEnv) confirmMinterContext(t *testing.T) context.Context {
	t.Helper()
	wsID := apptest.InstallationWorkspaceUUID(context.Background(), t, c.Pool)
	ctx := principal.WithWorkspaceID(context.Background(), wsID)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	user := ids.NewV7()
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"person": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}
