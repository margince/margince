// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func openAttachment(t *testing.T, e *apptest.AppEnv, id, accept string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, e.TS.URL+"/v1/attachments/"+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-Margince-Display-Locale", "de")
	resp, err := e.Client.Do(req)
	if err != nil {
		t.Fatalf("opening the attachment: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing the body: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading the body: %v", err)
	}
	return resp, string(body)
}

// A link to a missing attachment, opened in a tab, reads a sentence; the same
// request from a client reads the problem.
func TestAMissingAttachmentOpenedInATabReadsAPageAndAClientReadsTheProblem(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "File Page", "file-page@fable.test", "Admin")
	missing := ids.NewV7().String()

	tab, page := openAttachment(t, e, missing, "text/html,application/xhtml+xml,*/*;q=0.8")
	if tab.StatusCode != http.StatusNotFound || !strings.HasPrefix(tab.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("the tab got %d %q, want a 404 page", tab.StatusCode, tab.Header.Get("Content-Type"))
	}
	if !strings.Contains(page, "Diese Datei wurde gelöscht oder du hast keinen Zugriff mehr darauf.") {
		t.Errorf("the page does not say the file is gone in the reader's language:\n%s", page)
	}

	client, body := openAttachment(t, e, missing, "application/json")
	var problem struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &problem); err != nil || client.StatusCode != http.StatusNotFound || problem.Code != "not_found" {
		t.Fatalf("the client got %d %q %s, want the 404 not_found problem", client.StatusCode, client.Header.Get("Content-Type"), body)
	}
}
