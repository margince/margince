// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
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

func createForm(t *testing.T, e *apptest.AppEnv, body AnyMap) createdForm {
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
	form := createForm(t, e, AnyMap{
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

func TestASecondSubmissionFromAKnownEmailAddsToItsLead(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}})
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

func TestABrowserFormRefusesWhatItShouldNotLand(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, AnyMap{
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
	form := createForm(t, e, AnyMap{
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
	form := createForm(t, e, AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}})
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
	form := createForm(t, e, AnyMap{"name": "Website backend", "sender": "server"})
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
	form := createForm(t, e, AnyMap{"name": "Typeform", "sender": "typeform", "sender_secret": "tf-secret-of-some-length"})
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
	form := createForm(t, e, AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	fields := url.Values{"email": {"late@visitor.example"}}
	headers := map[string]string{"If-Match": strconv.FormatInt(form.Version, 10)}
	if status := e.Call(t, "PATCH", "/v1/web-forms/"+form.ID, AnyMap{"state": "paused"}, headers, nil); status != http.StatusOK {
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
	form := createForm(t, e, AnyMap{"name": "Backend", "sender": "server"})
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
