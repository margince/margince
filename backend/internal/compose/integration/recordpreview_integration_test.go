// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestRecordCardsArePublicButExposeOnlyTheirLimitedContext(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	cardClient := *e.TS.Client()
	cardClient.Jar = nil
	company := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Example Works", "domains": []AnyMap{{"domain": "example.test", "is_primary": true}}, "description": "PRIVATE-COMPANY-NOTES", "source": "manual"})
	stages := apptest.DiscoverSeededPipeline(t, e)
	deal := createdID(t, e, "/v1/deals", AnyMap{"name": "Renewal <Q> & 海", "description": "PRIVATE-DEAL-NOTES", "company_id": company, "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual"})
	contact := createdID(t, e, "/v1/contacts", AnyMap{"full_name": "Demo Buyer", "company_id": company, "title": "Director", "emails": []AnyMap{{"email": "private-buyer@example.test", "email_type": "work", "is_primary": true}}, "source": "manual"})
	for _, tc := range []struct{ kind, id, title, context string }{
		{"deals", deal, "Margince · Deal: Renewal &lt;Q&gt; &amp; 海", "Example Works"},
		{"companies", company, "Margince · Company: Example Works", "example.test"},
		{"contacts", contact, "Margince · Contact: Demo Buyer", "Director · Example Works"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			response, err := cardClient.Get(e.TS.URL + "/v1/public/record-previews/" + tc.kind + "/" + tc.id)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := response.Body.Close(); err != nil {
					t.Errorf("close response: %v", err)
				}
			}()
			raw, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			body := string(raw)
			if response.StatusCode != http.StatusOK || !strings.Contains(body, `property="og:title" content="`+tc.title+`"`) || !strings.Contains(body, tc.context) {
				t.Fatalf("card: %d %s", response.StatusCode, body)
			}
			if !strings.Contains(response.Header.Get("Content-Type"), "text/html") || response.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("headers: %v", response.Header)
			}
			head, err := cardClient.Head(e.TS.URL + "/v1/public/record-previews/" + tc.kind + "/" + tc.id)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := head.Body.Close(); err != nil {
					t.Errorf("close HEAD response: %v", err)
				}
			}()
			headBody, err := io.ReadAll(head.Body)
			if err != nil {
				t.Fatal(err)
			}
			if len(headBody) != 0 {
				t.Fatal("HEAD has a body")
			}
			if head.StatusCode != http.StatusOK {
				t.Fatalf("HEAD card: %d", head.StatusCode)
			}

			for _, private := range []string{"PRIVATE-COMPANY-NOTES", "PRIVATE-DEAL-NOTES", "private-buyer@example.test", `<Q>`} {
				if strings.Contains(body, private) {
					t.Errorf("card exposes %q", private)
				}
			}
			if !strings.Contains(body, `href="/#/`+tc.kind+`/`+tc.id+`"`) {
				t.Error("card does not open the protected record")
			}
		})
	}
	privateResponse, err := cardClient.Get(e.TS.URL + "/v1/deals/" + deal)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := privateResponse.Body.Close(); err != nil {
			t.Errorf("close response: %v", err)
		}
	}()
	if privateResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("full deal without session: %d", privateResponse.StatusCode)
	}

	if status := e.Call(t, "DELETE", "/v1/contacts/"+contact, nil, nil, nil); status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("archive: %d", status)
	}
	archived, err := cardClient.Get(e.TS.URL + "/v1/public/record-previews/contacts/" + contact)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := archived.Body.Close(); err != nil {
			t.Errorf("close response: %v", err)
		}
	}()
	archivedBody, err := io.ReadAll(archived.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(archivedBody), `src="/open-record.js"`) || strings.Contains(string(archivedBody), `property="og:title"`) {
		t.Fatal(string(archivedBody))
	}
	if archived.StatusCode != http.StatusNotFound {
		t.Fatalf("archived preview: %d", archived.StatusCode)
	}
}

func TestRecordCardsRemainAvailableWhenRelatedCompanyIsArchived(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	company := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Former employer", "source": "manual"})
	stages := apptest.DiscoverSeededPipeline(t, e)
	deal := createdID(t, e, "/v1/deals", AnyMap{"name": "Active renewal", "company_id": company, "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual"})
	if status := e.Call(t, "DELETE", "/v1/companies/"+company, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("archive company: %d", status)
	}
	client := *e.TS.Client()
	client.Jar = nil
	response, err := client.Get(e.TS.URL + "/v1/public/record-previews/deals/" + deal)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close response: %v", err)
		}
	}()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "Active renewal") || strings.Contains(string(body), "Former employer") {
		t.Fatalf("card: %d %s", response.StatusCode, body)
	}
}
