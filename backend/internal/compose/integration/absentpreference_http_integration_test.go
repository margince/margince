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

// getPreferring reads path over the wire, with Prefer set when prefer is not empty.
func getPreferring(t *testing.T, e *apptest.AppEnv, path, prefer string) *http.Response {
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
	t.Cleanup(func() { apptest.CloseBody(t, res) })
	return res
}

func wantAnswer(t *testing.T, res *http.Response, status int, applied string) {
	t.Helper()
	path := res.Request.URL.Path
	if res.StatusCode != status {
		t.Fatalf("GET %s → %d, want %d", path, res.StatusCode, status)
	}
	if got := res.Header.Get("Preference-Applied"); got != applied {
		t.Errorf("GET %s: Preference-Applied %q, want %q", path, got, applied)
	}
	if got := res.Header.Values("Vary"); !containsToken(got, "Prefer") {
		t.Errorf("GET %s: Vary %q, want it to name Prefer", path, got)
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

	wantAnswer(t, getPreferring(t, e, "/v1/brief", "absent=204"), http.StatusNoContent, "absent=204")
	wantAnswer(t, getPreferring(t, e, "/v1/brief", "respond-async, ABSENT=\"204\""), http.StatusNoContent, "absent=204")
	wantAnswer(t, getPreferring(t, e, "/v1/brief", ""), http.StatusNotFound, "")
	wantAnswer(t, getPreferring(t, e, "/v1/brief", "return=minimal"), http.StatusNotFound, "")

	var company AnyMap
	mustCall(t, e, "POST", "/v1/companies", AnyMap{"display_name": "Never Checked", "source": "manual"}, http.StatusCreated, &company)
	companyID, ok := company["id"].(string)
	if !ok {
		t.Fatalf("the created company carries no id: %v", company)
	}
	wantAnswer(t, getPreferring(t, e, "/v1/companies/"+companyID+"/vat-check", "absent=204"), http.StatusNoContent, "absent=204")
	wantAnswer(t, getPreferring(t, e, "/v1/companies/"+companyID+"/vat-check", ""), http.StatusNotFound, "")
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
		wantAnswer(t, getPreferring(t, e, path, "absent=204"), http.StatusNotFound, "")
	}
}

// An operation that does not declare the preference neither varies on it nor
// honours it.
func TestAnUndeclaredReadIgnoresTheAbsentPreference(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	res := getPreferring(t, e, "/v1/contacts/"+ids.NewV7().String(), "absent=204")
	if res.StatusCode != http.StatusNotFound || res.Header.Get("Preference-Applied") != "" {
		t.Fatalf("an undeclared read → %d, Preference-Applied %q; want 404 and none",
			res.StatusCode, res.Header.Get("Preference-Applied"))
	}
	if containsToken(res.Header.Values("Vary"), "Prefer") {
		t.Errorf("an undeclared read varies on Prefer: %q", res.Header.Values("Vary"))
	}
}
