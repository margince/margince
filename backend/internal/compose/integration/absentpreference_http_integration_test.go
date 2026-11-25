// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// answerTo reads path over the wire, with Prefer set when prefer is not
// empty, and returns its status and headers.
func answerTo(t *testing.T, e *apptest.AppEnv, path, prefer string) (int, http.Header) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, e.TS.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if prefer != "" {
		req.Header.Set("Prefer", prefer)
	}
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer apptest.CloseBody(t, res)
	return res.StatusCode, res.Header
}

func wantAnswer(t *testing.T, e *apptest.AppEnv, path, prefer string, status int, applied string) {
	t.Helper()
	got, header := answerTo(t, e, path, prefer)
	if got != status {
		t.Fatalf("GET %s with Prefer %q → %d, want %d", path, prefer, got, status)
	}
	if value := header.Get("Preference-Applied"); value != applied {
		t.Errorf("GET %s: Preference-Applied %q, want %q", path, value, applied)
	}
	if !containsToken(header.Values("Vary"), "Prefer") {
		t.Errorf("GET %s: Vary %q, want it to name Prefer", path, header.Values("Vary"))
	}
}

func containsToken(values []string, token string) bool {
	for _, value := range values {
		for part := range strings.SplitSeq(value, ",") {
			if http.CanonicalHeaderKey(strings.TrimSpace(part)) == token {
				return true
			}
		}
	}
	return false
}

func TestAReadWithNothingYetAnswers204OnlyWhenTheClientPrefersIt(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	wantAnswer(t, e, "/v1/brief", "absent=204", http.StatusNoContent, "absent=204")
	wantAnswer(t, e, "/v1/brief", "respond-async, ABSENT=\"204\"", http.StatusNoContent, "absent=204")
	wantAnswer(t, e, "/v1/brief", "", http.StatusNotFound, "")
	wantAnswer(t, e, "/v1/brief", "return=minimal", http.StatusNotFound, "")

	var company AnyMap
	mustCall(t, e, "POST", "/v1/companies", AnyMap{"display_name": "Never Checked", "source": "manual"}, http.StatusCreated, &company)
	companyID, ok := company["id"].(string)
	if !ok {
		t.Fatalf("the created company carries no id: %v", company)
	}
	wantAnswer(t, e, "/v1/companies/"+companyID+"/vat-check", "absent=204", http.StatusNoContent, "absent=204")
	wantAnswer(t, e, "/v1/companies/"+companyID+"/vat-check", "", http.StatusNotFound, "")
}

// A company the reader cannot reach answers what a company that does not exist
// answers, preference or not: 204 is only ever "visible, and nothing yet".
func TestARowScopeMissStays404WhenTheClientPrefersAbsent(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	missing := ids.NewV7().String()
	for _, path := range []string{
		"/v1/companies/" + missing + "/vat-check",
		"/v1/companies/" + missing + "/partner",
		"/v1/dedupe/contact-cases/" + missing,
		"/v1/records/contact/" + missing + "/ownership-offer",
	} {
		wantAnswer(t, e, path, "absent=204", http.StatusNotFound, "")
	}
}

// An operation that does not declare the preference neither varies on it nor
// honours it.
func TestAnUndeclaredReadIgnoresTheAbsentPreference(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	status, header := answerTo(t, e, "/v1/contacts/"+ids.NewV7().String(), "absent=204")
	if status != http.StatusNotFound || header.Get("Preference-Applied") != "" {
		t.Fatalf("an undeclared read → %d, Preference-Applied %q; want 404 and none",
			status, header.Get("Preference-Applied"))
	}
	if containsToken(header.Values("Vary"), "Prefer") {
		t.Errorf("an undeclared read varies on Prefer: %q", header.Values("Vary"))
	}
}
