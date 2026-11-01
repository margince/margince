// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package webforms

// Website forms end to end. An admin sets a form up and an anonymous sender
// posts to its address. The post lands as a lead with a timeline note, through
// the real edge and the real lead and activity writers.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/pkg/extension"
)

const siteOrigin = "https://www.example.com"

type createdForm struct {
	ID            string  `json:"id"`
	FormKey       string  `json:"form_key"`
	Version       int64   `json:"version"`
	SigningSecret *string `json:"signing_secret"`
}

func setupFormsApp(t *testing.T) *apptest.AppEnv {
	t.Helper()
	e := apptest.SetupAppWithOptions(t, compose.WithKeyvault(keyvault.NewMemory()))
	e.BootstrapWorkspace(t)
	return e
}

func createForm(t *testing.T, e *apptest.AppEnv, body integration.AnyMap) createdForm {
	t.Helper()
	var out createdForm
	if status := e.Call(t, "POST", "/v1/web-forms", body, nil, &out); status != http.StatusCreated {
		t.Fatalf("create web form → %d", status)
	}
	return out
}

type formAnswer struct {
	Status int
	Header http.Header
	Body   string
}

// postForm sends a raw body to a form's address with no session. It follows no
// redirect, so the test asserts the answer a browser would act on.
func postForm(t *testing.T, e *apptest.AppEnv, method, key, contentType string, body []byte, headers map[string]string) formAnswer {
	t.Helper()
	req, err := http.NewRequest(method, e.TS.URL+"/v1/public/forms/"+key, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		Transport:     e.TS.Client().Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req) //nolint:bodyclose // closed by apptest.CloseBody below
	if err != nil {
		t.Fatal(err)
	}
	defer apptest.CloseBody(t, resp)
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return formAnswer{Status: resp.StatusCode, Header: resp.Header, Body: string(raw)}
}

func browserPost(t *testing.T, e *apptest.AppEnv, key string, fields url.Values, origin string) formAnswer {
	t.Helper()
	return postForm(t, e, "POST", key, "application/x-www-form-urlencoded", []byte(fields.Encode()),
		map[string]string{"Origin": origin, "Accept": "application/json"})
}

func leadsWithEmail(t *testing.T, e *apptest.AppEnv, email string) (count int, origin, source string) {
	t.Helper()
	err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*), coalesce(max(origin), ''), coalesce(max(source_system), '') FROM lead WHERE email = $1`, email).
		Scan(&count, &origin, &source)
	if err != nil {
		t.Fatal(err)
	}
	return count, origin, source
}

func notesOnLeadWithEmail(t *testing.T, e *apptest.AppEnv, email string) []string {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(), `
		SELECT a.body FROM activity a JOIN activity_link l ON l.activity_id = a.id
		  JOIN lead ld ON ld.id = l.lead_id
		 WHERE ld.email = $1 AND a.kind = 'note' AND a.source_system = 'web_form' ORDER BY a.created_at`, email)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var bodies []string
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return bodies
}

func TestABrowserFormLandsALeadWithItsMessageOnTheTimeline(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{
		"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin + "/contact"},
		"consent_required": true, "consent_wording": "You may contact me about my enquiry.", "consent_policy_version": "2026-10",
	})
	answer := browserPost(t, e, form.FormKey, url.Values{
		"your-name": {"Ada Lovelace"}, "your-email": {"ada@visitor.example"}, "your-message": {"Please call me"},
		"budget": {"10k"}, "consent": {"on"}, "utm_source": {"newsletter"},
	}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("browser post → %d %s", answer.Status, answer.Body)
	}
	if got := answer.Header.Get("Access-Control-Allow-Origin"); got != siteOrigin {
		t.Fatalf("Access-Control-Allow-Origin = %q, want the form's website so its script can read the answer", got)
	}
	if count, origin, source := leadsWithEmail(t, e, "ada@visitor.example"); count != 1 || origin != "website" || source != "web_form" {
		t.Fatalf("lead rows = %d, origin %q, source_system %q", count, origin, source)
	}
	notes := notesOnLeadWithEmail(t, e, "ada@visitor.example")
	if len(notes) != 1 {
		t.Fatalf("notes = %d, want 1", len(notes))
	}
	for _, want := range []string{"Please call me", "budget: 10k", "utm_source: newsletter", "You may contact me about my enquiry.", "2026-10"} {
		if !strings.Contains(notes[0], want) {
			t.Errorf("the note does not carry %q:\n%s", want, notes[0])
		}
	}
}

func TestAFormNoteAnswersTheSubmissionFieldByField(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{
		"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin},
		"consent_wording": "You may contact me.", "consent_policy_version": "2026-10",
	})
	answer := browserPost(t, e, form.FormKey, url.Values{
		"name": {"Ada Lovelace"}, "email": {"ada@visitor.example"}, "message": {"Please call me"},
		"budget": {"10k"}, "consent": {"on"}, "utm_source": {"newsletter"}, "page_url": {siteOrigin + "/pricing"},
	}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("browser post → %d %s", answer.Status, answer.Body)
	}
	var noteID string
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT id FROM activity WHERE source_system = 'web_form' AND kind = 'note'`).Scan(&noteID); err != nil {
		t.Fatal(err)
	}
	var note crmcontracts.Activity
	if status := e.Call(t, "GET", "/v1/activities/"+noteID, nil, nil, &note); status != http.StatusOK {
		t.Fatalf("GET the note → %d", status)
	}
	sent := note.WebForm
	if sent == nil {
		t.Fatal("the note answers no web_form, so a reader gets the fields only as text")
	}
	if sent.Email != "ada@visitor.example" || deref(sent.FullName) != "Ada Lovelace" || deref(sent.Message) != "Please call me" ||
		deref(sent.PageUrl) != siteOrigin+"/pricing" {
		t.Errorf("fields = email %q, name %q, message %q, page %q", sent.Email, deref(sent.FullName), deref(sent.Message), deref(sent.PageUrl))
	}
	wantExtra := []crmcontracts.WebFormValue{{Name: "budget", Value: "10k"}}
	wantCampaign := []crmcontracts.WebFormValue{{Name: "utm_source", Value: "newsletter"}}
	if !slices.Equal(sent.Extra, wantExtra) || !slices.Equal(sent.Campaign, wantCampaign) {
		t.Errorf("extra = %v, campaign = %v, want %v and %v", sent.Extra, sent.Campaign, wantExtra, wantCampaign)
	}
	if c := sent.Consent; c == nil || !c.Ticked || c.Wording != "You may contact me." || deref(c.PolicyVersion) != "2026-10" {
		t.Errorf("consent = %+v, want the ticked box with its wording and policy version", c)
	}
	// The consent proves itself: it names the form and the page it was given on.
	if c := sent.Consent; c != nil && (c.FormId == nil || c.FormId.String() != form.ID || deref(c.FormName) != "Contact" ||
		deref(c.PageUrl) != siteOrigin+"/pricing") {
		t.Errorf("consent names form %v %q and page %q, want %s \"Contact\" and the pricing page",
			c.FormId, deref(c.FormName), deref(c.PageUrl), form.ID)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func TestASecondSubmissionFromAKnownEmailAddsToItsLead(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	for i, message := range []string{"First question", "Second question"} {
		answer := browserPost(t, e, form.FormKey, url.Values{"email": {"bob@visitor.example"}, "message": {message}}, siteOrigin)
		if answer.Status != http.StatusAccepted {
			t.Fatalf("post %d → %d %s", i+1, answer.Status, answer.Body)
		}
	}
	if count, _, _ := leadsWithEmail(t, e, "bob@visitor.example"); count != 1 {
		t.Fatalf("leads = %d, want the second post on the first lead", count)
	}
	if notes := notesOnLeadWithEmail(t, e, "bob@visitor.example"); len(notes) != 2 {
		t.Fatalf("notes = %d, want one per submission", len(notes))
	}
}

func TestAnErasedAddressGetsTheReceiptAnyOtherPostGets(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	const erased, clean = "geloescht@visitor.example", "neu@visitor.example"
	var subject struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", integration.AnyMap{
		"full_name": "Die Betroffene", "source": "manual",
		"emails": []integration.AnyMap{{"email": erased, "email_type": "work", "is_primary": true}},
	}, nil, &subject); status != http.StatusCreated {
		t.Fatalf("creating the subject → %d", status)
	}
	eraseContact(t, e, subject.ID)

	erasedAnswer := browserPost(t, e, form.FormKey, url.Values{"email": {erased}, "message": {"Hello again"}}, siteOrigin)
	cleanAnswer := browserPost(t, e, form.FormKey, url.Values{"email": {clean}, "message": {"Hello"}}, siteOrigin)
	if erasedAnswer.Status != http.StatusAccepted || cleanAnswer.Status != http.StatusAccepted {
		t.Fatalf("the erased address → %d %s, a clean one → %d %s; want 202 for both",
			erasedAnswer.Status, erasedAnswer.Body, cleanAnswer.Status, cleanAnswer.Body)
	}
	if erasedAnswer.Body != cleanAnswer.Body {
		t.Fatalf("the erased answer %s differs from the clean answer %s", erasedAnswer.Body, cleanAnswer.Body)
	}
	if count, _, _ := leadsWithEmail(t, e, erased); count != 0 {
		t.Fatalf("the erased address landed %d lead(s), want none", count)
	}
	if count, _, _ := leadsWithEmail(t, e, clean); count != 1 {
		t.Fatalf("the clean address landed %d lead(s), want 1", count)
	}
	var kept int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM erasure_suppression WHERE kind = 'email' AND value_hash = $1`,
		storekit.SuppressionHash(erased)).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Fatalf("the post lifted the suppression: %d row(s) left, want 1", kept)
	}
}

// eraseContact erases a contact the way an admin fulfils an Art. 17 request.
func eraseContact(t *testing.T, e *apptest.AppEnv, contactID string) {
	t.Helper()
	var dsr struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/data-subject-requests", integration.AnyMap{
		"kind": "erasure", "subject_ref": contactID,
	}, nil, &dsr); status != http.StatusCreated {
		t.Fatalf("filing the erasure request → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/data-subject-requests/"+dsr.ID, integration.AnyMap{
		"status": "fulfilled", "resolution": "erased per Art. 17", "identity_check": "id_document",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("fulfilling the erasure request → %d", status)
	}
}

func TestACustomFieldNameFillsTheLeadAndAValueThatDoesNotFitStaysOnTheNote(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithKeyvault(keyvault.NewMemory()), compose.WithSchemaPool(integration.SchemaPool(t)))
	e.BootstrapWorkspace(t)
	budget := apptest.CreateCustomField(t, e, integration.AnyMap{"object": "lead", "label": "Budget", "type": "number", "source": "manual"})
	plan := apptest.CreateCustomField(t, e, integration.AnyMap{"object": "lead", "label": "Plan", "type": "picklist", "options": []string{"Team", "Business"}, "source": "manual"})
	form := createForm(t, e, integration.AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	answer := browserPost(t, e, form.FormKey, url.Values{
		"email": {"cf@visitor.example"}, budget: {"12000"}, plan: {"Enterprise"},
	}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("browser post → %d %s", answer.Status, answer.Body)
	}
	var gotBudget, gotPlan *string
	query := "SELECT " + pgx.Identifier{budget}.Sanitize() + "::text, " + pgx.Identifier{plan}.Sanitize() +
		" FROM lead WHERE email = $1"
	if err := e.Owner.QueryRow(context.Background(), query, "cf@visitor.example").Scan(&gotBudget, &gotPlan); err != nil {
		t.Fatal(err)
	}
	if gotBudget == nil || *gotBudget != "12000" || gotPlan != nil {
		t.Fatalf("%s = %v, %s = %v; want 12000 and no plan, since Enterprise is not one of its options", budget, gotBudget, plan, gotPlan)
	}
	notes := notesOnLeadWithEmail(t, e, "cf@visitor.example")
	if len(notes) != 1 || !strings.Contains(notes[0], plan+": Enterprise") {
		t.Fatalf("notes = %q, want the plan the lead could not take", notes)
	}
}

func TestABrowserFormRefusesWhatItShouldNotLand(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{
		"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin},
		"consent_required": true, "consent_wording": "You may contact me.",
	})
	cases := []struct {
		name   string
		fields url.Values
		origin string
		status int
		email  string
	}{
		{"another website", url.Values{"email": {"c1@visitor.example"}, "consent": {"on"}}, "https://evil.example", http.StatusForbidden, "c1@visitor.example"},
		{"no email", url.Values{"name": {"Nobody"}, "consent": {"on"}}, siteOrigin, http.StatusUnprocessableEntity, ""},
		{"consent not ticked", url.Values{"email": {"c2@visitor.example"}}, siteOrigin, http.StatusUnprocessableEntity, "c2@visitor.example"},
		{"a filled honeypot", url.Values{"email": {"c3@visitor.example"}, "consent": {"on"}, "_mrg_hp": {"x"}}, siteOrigin, http.StatusAccepted, "c3@visitor.example"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if answer := browserPost(t, e, form.FormKey, c.fields, c.origin); answer.Status != c.status {
				t.Fatalf("→ %d %s, want %d", answer.Status, answer.Body, c.status)
			}
			if c.email == "" {
				return
			}
			if count, _, _ := leadsWithEmail(t, e, c.email); count != 0 {
				t.Fatalf("a refused or dropped post landed %d leads", count)
			}
		})
	}
}

func TestAPlainHTMLPostIsSentToTheFormsRedirect(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{
		"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin},
		"redirect_url": "https://www.example.com/thanks",
	})
	answer := postForm(t, e, "POST", form.FormKey, "application/x-www-form-urlencoded",
		[]byte(url.Values{"email": {"page@visitor.example"}}.Encode()),
		map[string]string{"Origin": siteOrigin, "Accept": "text/html,application/xhtml+xml"})
	if answer.Status != http.StatusSeeOther || answer.Header.Get("Location") != "https://www.example.com/thanks" {
		t.Fatalf("plain post → %d Location %q", answer.Status, answer.Header.Get("Location"))
	}
}

func TestTheCORSPreflightAnswersOnlyTheFormsWebsite(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	own := postForm(t, e, "OPTIONS", form.FormKey, "", nil, map[string]string{"Origin": siteOrigin, "Access-Control-Request-Method": "POST"})
	if own.Status != http.StatusNoContent || own.Header.Get("Access-Control-Allow-Origin") != siteOrigin {
		t.Fatalf("own website preflight → %d, allow-origin %q", own.Status, own.Header.Get("Access-Control-Allow-Origin"))
	}
	if own.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("the preflight allows credentials; the intake reads no cookie and must never invite one")
	}
	foreign := postForm(t, e, "OPTIONS", form.FormKey, "", nil, map[string]string{"Origin": "https://evil.example", "Access-Control-Request-Method": "POST"})
	if foreign.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("a foreign website was allowed: %q", foreign.Header.Get("Access-Control-Allow-Origin"))
	}
}

// serverHeaders signs with wall time: the intake checks freshness against its
// own clock, which the drift lane does not move.
func serverHeaders(secret, key string, body []byte, nonce string) map[string]string {
	at := time.Now()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(extension.SigningPayload(extension.ScopeInbound, "web-form", key, at, nonce, body))
	return map[string]string{
		extension.InboundHeaderTimestamp: strconv.FormatInt(at.Unix(), 10),
		extension.InboundHeaderNonce:     nonce,
		extension.InboundHeaderSignature: "sha256=" + hex.EncodeToString(mac.Sum(nil)),
	}
}

func TestAServerFormTakesOnlySignedPostsAndLandsAReplayOnce(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Website backend", "sender": "server"})
	if form.SigningSecret == nil || !strings.HasPrefix(*form.SigningSecret, "mwf_") {
		t.Fatalf("a server form's create did not answer its secret: %+v", form)
	}
	body := []byte(`{"email":"srv@visitor.example","name":"Sam Server","message":"From the backend"}`)
	headers := serverHeaders(*form.SigningSecret, form.FormKey, body, "a1b2c3d4")
	for i := range 2 {
		if answer := postForm(t, e, "POST", form.FormKey, "application/json", body, headers); answer.Status != http.StatusAccepted {
			t.Fatalf("signed post %d → %d %s", i+1, answer.Status, answer.Body)
		}
	}
	if count, _, _ := leadsWithEmail(t, e, "srv@visitor.example"); count != 1 {
		t.Fatalf("leads = %d, want 1", count)
	}
	if notes := notesOnLeadWithEmail(t, e, "srv@visitor.example"); len(notes) != 1 {
		t.Fatalf("notes = %d, want the replayed nonce to land nothing new", len(notes))
	}

	forged := serverHeaders("not-the-secret", form.FormKey, body, "e5f6")
	if answer := postForm(t, e, "POST", form.FormKey, "application/json", body, forged); answer.Status != http.StatusUnauthorized {
		t.Fatalf("a post signed with another secret → %d, want 401", answer.Status)
	}
	if answer := postForm(t, e, "POST", form.FormKey, "application/json", body, nil); answer.Status != http.StatusUnauthorized {
		t.Fatalf("an unsigned post → %d, want 401", answer.Status)
	}

	var rotated createdForm
	if status := e.Call(t, "POST", "/v1/web-forms/"+form.ID+"/rotate-secret", nil, nil, &rotated); status != http.StatusOK || rotated.SigningSecret == nil {
		t.Fatalf("rotate → %d", status)
	}
	stale := serverHeaders(*form.SigningSecret, form.FormKey, body, "0a0b")
	if answer := postForm(t, e, "POST", form.FormKey, "application/json", body, stale); answer.Status != http.StatusUnauthorized {
		t.Fatalf("a post under the rotated-out secret → %d, want 401", answer.Status)
	}
}

func TestATypeformWebhookLandsWithItsOwnSignature(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Typeform", "sender": "typeform", "sender_secret": "tf-secret-of-some-length"})
	body := []byte(`{"event_id":"01TF","form_response":{"definition":{"fields":[{"id":"f1","title":"Name"}]},
		"answers":[{"type":"text","text":"Tia","field":{"id":"f1"}},{"type":"email","email":"tia@visitor.example","field":{"id":"f2"}}]}}`)
	mac := hmac.New(sha256.New, []byte("tf-secret-of-some-length"))
	mac.Write(body)
	signed := map[string]string{"Typeform-Signature": "sha256=" + base64.StdEncoding.EncodeToString(mac.Sum(nil))}
	if answer := postForm(t, e, "POST", form.FormKey, "application/json", body, signed); answer.Status != http.StatusAccepted {
		t.Fatalf("typeform post → %d %s", answer.Status, answer.Body)
	}
	if count, _, _ := leadsWithEmail(t, e, "tia@visitor.example"); count != 1 {
		t.Fatalf("leads = %d, want 1", count)
	}
}

func TestAPausedFormIsGoneAndAnArchivedOneIsAbsent(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	fields := url.Values{"email": {"late@visitor.example"}}
	headers := map[string]string{"If-Match": strconv.FormatInt(form.Version, 10)}
	if status := e.Call(t, "PATCH", "/v1/web-forms/"+form.ID, integration.AnyMap{"state": "paused"}, headers, nil); status != http.StatusOK {
		t.Fatalf("pause → %d", status)
	}
	if answer := browserPost(t, e, form.FormKey, fields, siteOrigin); answer.Status != http.StatusGone {
		t.Fatalf("paused form → %d, want 410", answer.Status)
	}
	if status := e.Call(t, "DELETE", "/v1/web-forms/"+form.ID, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("archive → %d", status)
	}
	if answer := browserPost(t, e, form.FormKey, fields, siteOrigin); answer.Status != http.StatusNotFound {
		t.Fatalf("archived form → %d, want 404", answer.Status)
	}
	if answer := browserPost(t, e, "short", fields, siteOrigin); answer.Status != http.StatusNotFound {
		t.Fatalf("a key that cannot be one → %d, want 404", answer.Status)
	}
}

func TestAFormNeverAnswersItsSecretOnARead(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Backend", "sender": "server"})
	var raw map[string]any
	if status := e.Call(t, "GET", "/v1/web-forms/"+form.ID, nil, nil, &raw); status != http.StatusOK {
		t.Fatalf("get → %d", status)
	}
	if _, leaked := raw["signing_secret"]; leaked {
		t.Fatal("a read answered the signing secret")
	}
	if raw["secret_configured"] != true {
		t.Fatalf("secret_configured = %v, want true", raw["secret_configured"])
	}
}

func adminID(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var id string
	if err := e.Owner.QueryRow(context.Background(), `SELECT id::text FROM app_user WHERE email = 'ada@example.com'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func leadOwnerOf(t *testing.T, e *apptest.AppEnv, email string) *string {
	t.Helper()
	var owner *string
	if err := e.Owner.QueryRow(context.Background(), `SELECT owner_id::text FROM lead WHERE email = $1`, email).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	return owner
}

func TestALeadFromAFormGoesToTheFormsOwner(t *testing.T) {
	e := setupFormsApp(t)
	owner := adminID(t, e)
	form := createForm(t, e, integration.AnyMap{
		"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}, "owner_id": owner,
	})
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"owned@visitor.example"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("browser post → %d %s", answer.Status, answer.Body)
	}
	if got := leadOwnerOf(t, e, "owned@visitor.example"); got == nil || *got != owner {
		t.Fatalf("lead owner = %v, want the form's owner %s", got, owner)
	}
}

func TestALeadFromAFormWhoseOwnerLeftLandsUnassigned(t *testing.T) {
	e := setupFormsApp(t)
	owner := adminID(t, e)
	form := createForm(t, e, integration.AnyMap{
		"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}, "owner_id": owner,
	})
	if _, err := e.Owner.Exec(context.Background(), `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, owner); err != nil {
		t.Fatal(err)
	}
	answer := browserPost(t, e, form.FormKey, url.Values{"email": {"orphan@visitor.example"}}, siteOrigin)
	if answer.Status != http.StatusAccepted {
		t.Fatalf("browser post → %d %s; a departed owner must not cost the visitor their enquiry", answer.Status, answer.Body)
	}
	if got := leadOwnerOf(t, e, "orphan@visitor.example"); got != nil {
		t.Fatalf("lead owner = %s, want unassigned once the form's owner left", *got)
	}
}

type formLastPost struct {
	Version    int64      `json:"version"`
	LastPostAt *time.Time `json:"last_post_at"`
}

func readFormLastPost(t *testing.T, e *apptest.AppEnv, id string) formLastPost {
	t.Helper()
	var out formLastPost
	if status := e.Call(t, "GET", "/v1/web-forms/"+id, nil, nil, &out); status != http.StatusOK {
		t.Fatalf("read web form → %d", status)
	}
	return out
}

// A form's row says whether its pasted code works. A dropped post is no sign
// of that, and a landed one leaves the version an admin's edit holds alone.
func TestAFormKnowsWhenItLastLandedAPost(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, integration.AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	if got := readFormLastPost(t, e, form.ID); got.LastPostAt != nil {
		t.Fatalf("a new form reads a last post at %s, want none", got.LastPostAt)
	}
	browserPost(t, e, form.FormKey, url.Values{"email": {"bot@visitor.example"}, "_mrg_hp": {"x"}}, siteOrigin)
	if got := readFormLastPost(t, e, form.ID); got.LastPostAt != nil {
		t.Fatalf("a post the spam checks dropped stamped a last post at %s", got.LastPostAt)
	}
	if answer := browserPost(t, e, form.FormKey, url.Values{"email": {"ada@visitor.example"}}, siteOrigin); answer.Status != http.StatusAccepted {
		t.Fatalf("browser post → %d %s", answer.Status, answer.Body)
	}
	got := readFormLastPost(t, e, form.ID)
	if got.LastPostAt == nil {
		t.Fatal("a landed post left the form with no last post")
	}
	if got.Version != form.Version {
		t.Fatalf("a landed post moved the form's version from %d to %d, which fails an admin's edit in progress", form.Version, got.Version)
	}
}
