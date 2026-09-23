// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/websearch"
)

// The deny-list is the seam's load-bearing promise, so it is asserted rather
// than trusted: a regression here is the difference between a product that
// cites LinkedIn and one that scrapes it.
func TestMayFetchRefusesTheAuthWalledPlatforms(t *testing.T) {
	refused := []string{
		"https://www.linkedin.com/in/anna-weber",
		"https://linkedin.com/in/anna-weber",
		"https://de.linkedin.com/in/anna-weber",
		"https://www.xing.com/profile/Anna_Weber",
		"http://facebook.com/someone",
		// The fully-qualified spellings. DNS resolves these to the same
		// servers, so a policy that reads them as different hosts is a
		// bypass rather than a nicety.
		"https://linkedin.com./in/anna-weber",
		"https://WWW.LinkedIn.COM./in/anna-weber",
		"https://de.linkedin.com../in/anna-weber",
	}
	for _, raw := range refused {
		if d := MayFetch(raw); d.Allowed {
			t.Errorf("MayFetch(%q) allowed the fetch; the platform is deny-listed", raw)
		}
	}
}

// ADR-0081 forbids a credentialed fetch outright. A URL carrying userinfo
// would have the fetcher present somebody's password to a site this product
// has no account with, so it is refused before the host is even considered.
func TestMayFetchRefusesAnAddressCarryingCredentials(t *testing.T) {
	for _, raw := range []string{
		"https://placeholder-user:placeholder-pass@scalecommerce.example/team",
		"https://placeholder-user@scalecommerce.example/team",
		"http://placeholder-user:placeholder-pass@example.de/impressum",
	} {
		if d := MayFetch(raw); d.Allowed {
			t.Errorf("MayFetch(%q) allowed a credentialed fetch", raw)
		}
	}
}

func TestMayFetchAllowsAnOrdinaryPublicPage(t *testing.T) {
	allowed := []string{
		"https://scalecommerce.example/team",
		"https://www.neuland-bfi.de/de/ueber-uns/partner",
		"http://example.de/impressum",
	}
	for _, raw := range allowed {
		d := MayFetch(raw)
		if !d.Allowed {
			t.Errorf("MayFetch(%q) refused a public page: %s", raw, d.Reason)
		}
	}
}

// A URL the policy cannot decompose is refused rather than passed through:
// guessing here fetches the thing the policy exists to avoid.
func TestMayFetchRefusesWhatItCannotParse(t *testing.T) {
	for _, raw := range []string{"", "   ", "not a url", "ftp://example.com/x", "mailto:a@b.c"} {
		if d := MayFetch(raw); d.Allowed {
			t.Errorf("MayFetch(%q) allowed an address it could not apply policy to", raw)
		}
	}
}

// Denied for fetching and citable are different questions. A LinkedIn URL in
// a search result is where the claim lives, and saying so costs nobody
// anything — throwing it away would discard the metadata that makes
// discovery useful without a fetch.
// Here rather than in the websearch port because the two halves now live in
// two packages, and only this one may reach both: platform may import shared,
// not the other way round. The assertion is the same one — losing it to the
// move would have left the distinction stated in prose and held by nothing.
func TestADeniedHostIsStillCitable(t *testing.T) {
	r := websearch.Result{URL: "https://www.linkedin.com/in/anna-weber", Title: "Anna Weber — Head of Procurement"}
	if MayFetch(r.URL).Allowed {
		t.Fatal("the fixture is wrong: this host must be deny-listed for fetching")
	}
	if !websearch.Citable(r) {
		t.Error("a deny-listed host must still be citable — the URL is evidence of where the claim appears")
	}
}

// The fetcher refuses a denied platform WITHOUT asking it anything.
//
// The ordering is the substance of the guard, not a detail of it: fetching a
// denied platform's robots.txt is itself a request to that platform, and
// "never fetched" has to mean never. A policy applied after robots would still
// refuse the page while having already announced us to the host — and on a
// platform serving no robots.txt, or one whose robots permits the path, the
// old arrangement refused nothing at all.
//
// Proved against a local server standing in for a denied host, so the
// assertion is about what was REQUESTED rather than about a returned error
// that a network failure could also produce.
func TestTheFetcherRefusesADeniedHostWithoutAskingItForRobots(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		//craft:ignore swallowed-errors httptest handler write; a failed write fails the test through the assertion below
		_, _ = w.Write([]byte("User-agent: *\nAllow: /\n"))
	}))
	defer srv.Close()

	// The host this server answers on, denied for the length of this test. Its
	// robots ALLOWS everything, so anything that reaches the site is fetched —
	// which is what makes a silent policy visible here.
	// The HOSTNAME, without the port: that is what MayFetch compares, and a
	// deny entry carrying the port matches nothing and quietly passes.
	at, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parsing the test server's address: %v", err)
	}
	restore := deniedHosts
	deniedHosts = append(append([]string{}, deniedHosts...), normalizeHost(at.Hostname()))
	defer func() { deniedHosts = restore }()

	if _, err := testFetcher().Fetch(context.Background(), srv.URL+"/team"); !errors.Is(err, ErrFetchPolicy) {
		t.Fatalf("fetching a denied host = %v, want ErrFetchPolicy", err)
	}
	if len(asked) != 0 {
		t.Errorf("the denied host was sent %v — a refusal that first announces us to the platform "+
			"is not the promise this policy makes", asked)
	}
}

// And the refusal is OURS, not the site's. An operator reading a run record
// has to be able to tell a platform we will not read from a platform that
// asked not to be read: the two are different promises to different parties,
// and only one of them is ours to change.
func TestTheProductsRefusalIsDistinctFromTheSites(t *testing.T) {
	if errors.Is(ErrFetchPolicy, ErrRobotsDisallowed) || errors.Is(ErrRobotsDisallowed, ErrFetchPolicy) {
		t.Error("the two refusals are indistinguishable, so a run record cannot say which one fired")
	}
}
