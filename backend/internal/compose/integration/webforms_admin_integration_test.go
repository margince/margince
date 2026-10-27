// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Website forms as an admin keeps them: the list, a builder's pasted secret
// replaced in place, and the settings a save refuses.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestTheFormListShowsArchivedFormsOnlyWhenAskedTo(t *testing.T) {
	e := setupFormsApp(t)
	createForm(t, e, AnyMap{"name": "Live", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	retired := createForm(t, e, AnyMap{"name": "Retired", "sender": "server"})
	if status := e.Call(t, "DELETE", "/v1/web-forms/"+retired.ID, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("archive → %d", status)
	}
	names := func(path string) []string {
		var page struct {
			Data []struct {
				Name string `json:"name"`
			} `json:"data"`
			SecretsAvailable bool `json:"secrets_available"`
		}
		if status := e.Call(t, "GET", path, nil, nil, &page); status != http.StatusOK || !page.SecretsAvailable {
			t.Fatalf("GET %s → %d, secrets_available %v", path, status, page.SecretsAvailable)
		}
		out := make([]string, 0, len(page.Data))
		for _, f := range page.Data {
			out = append(out, f.Name)
		}
		return out
	}
	if got := names("/v1/web-forms"); len(got) != 1 || got[0] != "Live" {
		t.Fatalf("live list = %v, want only the live form", got)
	}
	if got := names("/v1/web-forms?include_archived=true"); len(got) != 2 {
		t.Fatalf("list with archived = %v, want both forms", got)
	}
}

func tallyHeaders(secret string, body []byte) map[string]string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return map[string]string{"Tally-Signature": base64.StdEncoding.EncodeToString(mac.Sum(nil))}
}

func TestABuilderSecretPastedAgainReplacesTheOldOne(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, AnyMap{"name": "Tally", "sender": "tally", "sender_secret": "tally-secret-number-one"})
	body := []byte(`{"eventId":"ta-1","eventType":"FORM_RESPONSE","data":{"fields":[
		{"key":"q1","label":"Email","type":"INPUT_EMAIL","value":"tam@visitor.example"},
		{"key":"q2","label":"Tools","type":"CHECKBOXES","value":["o2"],"options":[{"id":"o1","text":"CRM"},{"id":"o2","text":"Mail"}]}]}}`)
	if answer := postForm(t, e, "POST", form.FormKey, "application/json", body, tallyHeaders("tally-secret-number-one", body)); answer.Status != http.StatusAccepted {
		t.Fatalf("tally post → %d %s", answer.Status, answer.Body)
	}
	if notes := notesOnLeadWithEmail(t, e, "tam@visitor.example"); len(notes) != 1 || !strings.Contains(notes[0], "Tools: Mail") {
		t.Fatalf("notes = %q, want the ticked option by its text", notes)
	}

	headers := map[string]string{"If-Match": strconv.FormatInt(form.Version, 10)}
	if status := e.Call(t, "PATCH", "/v1/web-forms/"+form.ID, AnyMap{"sender_secret": "tally-secret-number-two"}, headers, nil); status != http.StatusOK {
		t.Fatalf("replace secret → %d", status)
	}
	again := []byte(`{"eventId":"ta-2","eventType":"FORM_RESPONSE","data":{"fields":[
		{"key":"q1","label":"Email","type":"INPUT_EMAIL","value":"tam@visitor.example"}]}}`)
	if answer := postForm(t, e, "POST", form.FormKey, "application/json", again, tallyHeaders("tally-secret-number-one", again)); answer.Status != http.StatusUnauthorized {
		t.Fatalf("a post under the replaced secret → %d, want 401", answer.Status)
	}
	if answer := postForm(t, e, "POST", form.FormKey, "application/json", again, tallyHeaders("tally-secret-number-two", again)); answer.Status != http.StatusAccepted {
		t.Fatalf("a post under the new secret → %d %s", answer.Status, answer.Body)
	}
}

func TestATallyFormSetUpWithoutASecretGetsOneMintedToTypeIntoTally(t *testing.T) {
	e := setupFormsApp(t)
	form := createForm(t, e, AnyMap{"name": "Tally", "sender": "tally"})
	if form.SigningSecret == nil || *form.SigningSecret == "" {
		t.Fatal("create answered no secret to type into Tally")
	}
	body := []byte(`{"eventId":"ta-1","eventType":"FORM_RESPONSE","data":{"fields":[
		{"key":"q1","label":"Email","type":"INPUT_EMAIL","value":"tia@visitor.example"}]}}`)
	if answer := postForm(t, e, "POST", form.FormKey, "application/json", body, tallyHeaders(*form.SigningSecret, body)); answer.Status != http.StatusAccepted {
		t.Fatalf("a post under the minted secret → %d %s", answer.Status, answer.Body)
	}

	var rotated createdForm
	if status := e.Call(t, "POST", "/v1/web-forms/"+form.ID+"/rotate-secret", nil, nil, &rotated); status != http.StatusOK || rotated.SigningSecret == nil {
		t.Fatalf("rotate → %d, secret %v", status, rotated.SigningSecret)
	}
	again := []byte(`{"eventId":"ta-2","eventType":"FORM_RESPONSE","data":{"fields":[
		{"key":"q1","label":"Email","type":"INPUT_EMAIL","value":"tia@visitor.example"}]}}`)
	if answer := postForm(t, e, "POST", form.FormKey, "application/json", again, tallyHeaders(*form.SigningSecret, again)); answer.Status != http.StatusUnauthorized {
		t.Fatalf("a post under the rotated-out secret → %d, want 401", answer.Status)
	}
}

func TestAWebflowFormWaitsForTheSecretWebflowIssues(t *testing.T) {
	e := setupFormsApp(t)
	var form struct {
		createdForm
		SecretConfigured bool `json:"secret_configured"`
	}
	if status := e.Call(t, "POST", "/v1/web-forms", AnyMap{"name": "Webflow", "sender": "webflow"}, nil, &form); status != http.StatusCreated {
		t.Fatalf("create before Webflow issued its secret → %d", status)
	}
	if form.SecretConfigured || form.SigningSecret != nil {
		t.Fatalf("created = %+v, want no secret: Webflow issues it", form)
	}
	body := []byte(`{"triggerType":"form_submission","payload":{"id":"wf-1","data":{"Email":"wim@visitor.example"}}}`)
	if answer := postForm(t, e, "POST", form.FormKey, "application/json", body, nil); answer.Status != http.StatusUnauthorized {
		t.Fatalf("a post before the secret is pasted → %d, want 401", answer.Status)
	}
	headers := map[string]string{"If-Match": strconv.FormatInt(form.Version, 10)}
	if status := e.Call(t, "PATCH", "/v1/web-forms/"+form.ID, AnyMap{"sender_secret": "webflow-issued-secret-1"}, headers, &form); status != http.StatusOK || !form.SecretConfigured {
		t.Fatalf("paste Webflow's secret → %d, secret_configured %v", status, form.SecretConfigured)
	}
	if status := e.Call(t, "POST", "/v1/web-forms/"+form.ID+"/rotate-secret", nil, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("rotating a Webflow secret → %d, want 422: Webflow issues it", status)
	}
}

func TestASaveNamingAnUnknownLeadOriginIsRefused(t *testing.T) {
	e := setupFormsApp(t)
	body := AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}, "lead_origin": "no-such-origin"}
	if status := e.Call(t, "POST", "/v1/web-forms", body, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("create → %d, want 422", status)
	}
	form := createForm(t, e, AnyMap{"name": "Contact", "sender": "browser", "allowed_origins": []string{siteOrigin}})
	headers := map[string]string{"If-Match": strconv.FormatInt(form.Version, 10)}
	if status := e.Call(t, "PATCH", "/v1/web-forms/"+form.ID, AnyMap{"lead_origin": "no-such-origin"}, headers, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("update → %d, want 422", status)
	}
}
