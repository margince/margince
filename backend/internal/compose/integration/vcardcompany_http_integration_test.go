// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The vCard import over HTTP, from one company's Contacts tab: the company
// rides in the query, and each contact the file creates works for it.

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// importCardsOver posts a .vcf as the multipart form the browser sends, to path.
func importCardsOver(t *testing.T, e *apptest.AppEnv, path, vcf string) (int, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "cards.vcf")
	if err != nil {
		t.Fatalf("building the multipart body: %v", err)
	}
	if _, err := part.Write([]byte(vcf)); err != nil {
		t.Fatalf("writing the file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing the multipart writer: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, e.TS.URL+path, &body)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatalf("uploading: %v", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("closing the response body: %v", cerr)
		}
	}()
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decoding the answer: %v", err)
	}
	return resp.StatusCode, string(raw)
}

func TestTheVCardImportFilesCreatedContactsUnderTheCompanyNamedInTheQuery(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "VCard Company", "vcard-company-a@fable.test", "Admin")
	company := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Atrium Home GmbH"})
	card := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Ada Atrium\r\nORG:Elsewhere AG\r\nEMAIL;TYPE=work:ada@atrium-home.example\r\nEND:VCARD\r\n"

	status, answer := importCardsOver(t, e, "/v1/contacts/vcard-import?company_id="+company, card)

	if status != http.StatusOK {
		t.Fatalf("the import answered %d %s", status, answer)
	}
	if n := countOwned(t, e, `SELECT count(*) FROM relationship r JOIN contact c ON c.id = r.contact_id
		WHERE r.kind = 'employment' AND r.company_id = $1 AND c.full_name = 'Ada Atrium' AND r.archived_at IS NULL`, company); n != 1 {
		t.Errorf("%d employment edges join the created contact to the company, want one", n)
	}
	if n := countOwned(t, e, `SELECT count(*) FROM company WHERE display_name = 'Elsewhere AG'`); n != 0 {
		t.Errorf("the card's own company was created %d times", n)
	}
}

func TestTheVCardImportRefusesACompanyNobodyHasAndWritesNothing(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "VCard Company", "vcard-company-b@fable.test", "Admin")
	card := "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Dina Dangling\r\nEMAIL;TYPE=work:dina@dangling.example\r\nEND:VCARD\r\n"

	status, answer := importCardsOver(t, e, "/v1/contacts/vcard-import?company_id=018f0000-0000-7000-8000-000000000001", card)

	if status != http.StatusUnprocessableEntity {
		t.Fatalf("the import answered %d %s, want 422", status, answer)
	}
	if n := countOwned(t, e, `SELECT count(*) FROM contact WHERE full_name = 'Dina Dangling'`); n != 0 {
		t.Errorf("a refused import left %d contacts", n)
	}
}
