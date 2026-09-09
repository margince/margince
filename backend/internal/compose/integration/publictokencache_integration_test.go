// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Every answer on the two anonymous token prefixes is uncacheable.
//
// /v1/public/preferences/* and /v1/public/confirm/* derive their whole
// response from a bearer token that lives in somebody's mailbox for weeks. A
// shared cache holding ANY of those answers hands the next reader through that
// cache somebody else's consent state or somebody else's record. Both edges set
// Cache-Control: no-store before their first refusal for that reason, and until
// this file nothing asserted it: deleting either line left the suite green.
//
// So the assertion is a census, not a spot check. Each case below drives a real
// request through the real stack and every one of them is checked, because the
// header's whole value is that it holds on the answers nobody thought about —
// the rate-limited one, the not-found one, the wrong-verb one. A test that
// checked only the happy path would pass while the refusals leaked.
//
// This file covers the answers the ROUTER produces, over live tokens in every
// state one can be in. The answers produced ABOVE the router — the session
// middleware's, which reply before any of this is reached — are covered by
// compose/credentialpathcache_test.go, against the single middleware that now
// sets the header for both. Splitting them that way is deliberate: one claim is
// about real token states and needs a database, the other is about ordering and
// must not.
//
// The routes this file claims to cover are declared below, one COVERS line each.
// gates/publictokencachecensus_test.go reads those lines and compares them with
// what backend/api/crm.yaml actually publishes under the two prefixes, in both
// directions. Publish a sixth route without a case here and that gate fails, so
// the census cannot quietly shrink to cover less than it says it does.
//
//   COVERS: GET /public/preferences/{token}
//   COVERS: PUT /public/preferences/{token}
//   COVERS: POST /public/preferences/{token}/unsubscribe
//   COVERS: GET /public/confirm/{token}
//   COVERS: POST /public/confirm/{token}

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/capabilitypath"
)

// tokenCase is one answer the surface can give, named for the state that
// produces it so a failure says which answer leaked rather than which line did.
type tokenCase struct {
	name   string
	method string
	path   string
	body   any
}

// assertNoStore drives every case and reports EVERY failure, not the first.
// A partial answer here would send somebody back for a second run to discover
// the next uncacheable response they also lost.
// It returns the contract operations the cases actually drove, so a test can
// prove its COVERS claim against its own table rather than beside it.
func assertNoStore(t *testing.T, e *apptest.AppEnv, cases []tokenCase) map[string]bool {
	t.Helper()
	driven := make(map[string]bool, len(cases))
	for _, tc := range cases {
		driven[tc.method+" "+strings.TrimPrefix(redactToken(tc.path), "/v1")] = true
		status, headers := publicCallWithHeaders(t, e, tc.method, tc.path, tc.body, nil, nil)
		if got := headers.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: %s %s → %d carried Cache-Control %q, want %q — this answer is "+
				"derived from a mailed bearer token and a shared cache holding it is a leak",
				tc.name, tc.method, redactToken(tc.path), status, got, "no-store")
		}
	}
	return driven
}

// assertClaimed proves this file's COVERS declarations are true of its own case
// tables. The gate compares those declarations with the contract; this compares
// them with what the tests really drive. Without it the declarations could drift
// into a claim no request backs, and the gate would keep passing on the strength
// of a comment.
//
// The claims are READ from the COVERS lines rather than restated as an argument.
// A hand-passed list would be a third copy: add a route, a COVERS line and no
// case, and a restated list would keep checking the old set while both the gate
// and this assertion reported success.
func assertClaimed(t *testing.T, driven map[string]bool, prefix string) {
	t.Helper()
	claimed := declaredCovers(t, prefix)
	if len(claimed) == 0 {
		t.Fatalf("no COVERS line names %s, so this assertion has no subject and would pass "+
			"however little the cases drove", prefix)
	}
	for _, op := range claimed {
		if !driven[op] {
			t.Errorf("this file declares COVERS %s but no case drove it — the declaration the "+
				"census gate reads is not backed by a request", op)
		}
	}
}

// declaredCovers reads this file's own COVERS lines, keeping those naming the
// given path prefix. Reading the source is what makes the declaration and the
// cases one claim rather than two that can disagree.
func declaredCovers(t *testing.T, prefix string) []string {
	t.Helper()
	raw, err := os.ReadFile("publictokencache_integration_test.go")
	if err != nil {
		t.Fatalf("reading this file's own COVERS declarations: %v", err)
	}
	var claimed []string
	for _, m := range regexp.MustCompile(`(?m)^//\s+COVERS:\s+([A-Z]+)\s+(\S+)\s*$`).
		FindAllStringSubmatch(string(raw), -1) {
		if strings.HasPrefix(m[2], prefix) {
			claimed = append(claimed, m[1]+" "+m[2])
		}
	}
	return claimed
}

// redactToken keeps a live token out of the test log and, in doing so, turns a
// driven URL back into the contract path it exercised, which is what lets a test
// check its own COVERS claim against what it really drives.
//
// The prefix list and the replacement both come from capabilitypath, which
// already decides which segments are credentials. A second copy here would drift
// the first time a prefix is added there — and the placeholder is normalized to
// the contract's own {token} spelling rather than re-deciding anything.
func redactToken(path string) string {
	withoutQuery := strings.SplitN(path, "?", 2)[0]
	return strings.Replace(capabilitypath.Redact(withoutQuery), "[redacted]", "{token}", 1)
}

// The preference edge, across every state its token can be in.
func TestEveryPreferenceAnswerIsUncacheable(t *testing.T) {
	c := setupConsent(t)
	grantPurpose(t, c, createNewsletterPurpose(t, c))
	live := sendAndAssertUnsubscribeLink(t, c)

	driven := assertNoStore(t, c.AppEnv, []tokenCase{
		{"a live token's view", "GET", "/v1/public/preferences/" + live, nil},
		{
			"a live token's one-click withdrawal", "POST",
			"/v1/public/preferences/" + live + "/unsubscribe?purpose=newsletter", nil,
		},
		{
			"a live token's saved choices", "PUT", "/v1/public/preferences/" + live,
			AnyMap{"choices": []AnyMap{{
				"purpose_key": "newsletter", "state": "granted",
				"wording": "Yes, you may contact me about this.",
			}}},
		},
		{
			"a refused save", "PUT", "/v1/public/preferences/" + live,
			AnyMap{"choices": []AnyMap{}},
		},
		{"an unknown token", "GET", "/v1/public/preferences/pref_does_not_exist", nil},
		{
			"an unknown token's withdrawal", "POST",
			"/v1/public/preferences/pref_does_not_exist/unsubscribe?purpose=newsletter", nil,
		},
		{"an empty token", "GET", "/v1/public/preferences/", nil},
		{"a GET on the one-click verb", "GET", "/v1/public/preferences/" + live + "/unsubscribe", nil},
	})
	assertClaimed(t, driven, "/public/preferences/")

	// A revoked token answers as absent, and that answer is a fact about
	// somebody too: it says a token that once existed no longer does.
	revokePreferenceTokens(t, c)
	assertNoStore(t, c.AppEnv, []tokenCase{
		{"a revoked token", "GET", "/v1/public/preferences/" + live, nil},
		{
			"a revoked token's withdrawal", "POST",
			"/v1/public/preferences/" + live + "/unsubscribe?purpose=newsletter", nil,
		},
	})
}

// The confirm edge, across every state its token can be in.
//
// Its GET discloses the person's stored record rather than a list of switches,
// so a cached answer here is a larger disclosure than on the preference edge.
func TestEveryConfirmAnswerIsUncacheable(t *testing.T) {
	c := setupConsent(t)

	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent/confirm-request",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("ask the workspace to mail the confirm link → %d", status)
	}
	live := confirmLinkToken(t, c.AppEnv)

	driven := assertNoStore(t, c.AppEnv, []tokenCase{
		{"a live link's record view", "GET", "/v1/public/confirm/" + live, nil},
		{"an unknown token", "GET", "/v1/public/confirm/confirm_does_not_exist", nil},
		{
			"an unknown token's answer", "POST", "/v1/public/confirm/confirm_does_not_exist",
			AnyMap{"marketing_choice": "granted", "marketing_wording": "Yes."},
		},
		{"an empty token", "GET", "/v1/public/confirm/", nil},
	})
	assertClaimed(t, driven, "/public/confirm/")

	// Spending the link is the one answer the subject actually acts on, and
	// the spent link's refusal that follows it.
	assertNoStore(t, c.AppEnv, []tokenCase{
		{
			"the subject spending their link", "POST", "/v1/public/confirm/" + live,
			AnyMap{"marketing_choice": "granted", "marketing_wording": "Yes, send me product news."},
		},
		{"a consumed link", "GET", "/v1/public/confirm/" + live, nil},
		{
			"a consumed link replayed", "POST", "/v1/public/confirm/" + live,
			AnyMap{"marketing_choice": "granted", "marketing_wording": "Yes, send me product news."},
		},
	})

	// An expired link is aged in the database rather than waited for: the TTL
	// is days, and a real-clock test would be a flake by construction.
	expired := freshExpiredConfirmToken(t, c)
	assertNoStore(t, c.AppEnv, []tokenCase{
		{"an expired link", "GET", "/v1/public/confirm/" + expired, nil},
		{
			"an expired link answered", "POST", "/v1/public/confirm/" + expired,
			AnyMap{"marketing_choice": "granted", "marketing_wording": "Yes."},
		},
	})
}

// The answer the session middleware gives before either edge is reached.
//
// This is the leak that shipped: identity.Handlers.Middleware wraps the public
// edges and answers a not-bootstrapped installation with a 503 of its own, so
// while each edge set the header itself, that answer carried none. No test could
// see it, because every harness in the tree bootstraps a workspace first — the
// state that leaked is the one no test environment produces.
//
// So this one deliberately does not bootstrap. It is also the case that proves
// the header is wired ABOVE the session middleware rather than below it: nest
// the wrapper back under and this fails while every hand-built unit case in
// compose/credentialpathcache_test.go still passes.
func TestANotBootstrappedInstallationAnswersUncacheable(t *testing.T) {
	e := apptest.SetupAppWithOptions(t)
	// No BootstrapWorkspaceSession: the installation has no active
	// company, which is what makes the session middleware answer first.

	for _, path := range []string{
		"/v1/public/confirm/sometoken",
		"/v1/public/preferences/sometoken",
	} {
		status, headers := publicCallWithHeaders(t, e, "GET", path, nil, nil, nil)
		if status != http.StatusServiceUnavailable {
			t.Fatalf("%s → %d, want 503 — this case did not reach the answer the session "+
				"middleware gives, so its header assertion proves nothing",
				capabilitypath.Redact(path), status)
		}
		if got := headers.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: the not-bootstrapped answer carried Cache-Control %q, want %q — it is "+
				"given above both public edges, so a header either edge sets never reaches it",
				capabilitypath.Redact(path), got, "no-store")
		}
	}
}

// Both edges refuse a flood before they reach any handler, and that refusal is
// the answer most likely to be served from a cache in front of the API — it is
// cheap, repeated, and identical. It is also the one a reviewer would not think
// to check, which is exactly why it is here.
func TestARateLimitedAnswerIsUncacheable(t *testing.T) {
	c := setupConsent(t)

	for _, edge := range []struct{ name, path string }{
		{"the preference edge", "/v1/public/preferences/pref_flood/unsubscribe?purpose=newsletter"},
		{"the confirm edge", "/v1/public/confirm/confirm_flood"},
	} {
		var status int
		var headers http.Header
		// The per-token brake is 20 a minute on both edges; this outruns it.
		for i := 0; i < 40 && status != http.StatusTooManyRequests; i++ {
			status, headers = publicCallWithHeaders(t, c.AppEnv, "POST", edge.path,
				AnyMap{"marketing_choice": "granted", "marketing_wording": "Yes."}, nil, nil)
		}
		if status != http.StatusTooManyRequests {
			t.Fatalf("%s never throttled (last %d), so this test asserts nothing about a "+
				"throttled answer", edge.name, status)
		}
		if got := headers.Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: the throttled answer carried Cache-Control %q, want %q",
				edge.name, got, "no-store")
		}
	}
}

// revokePreferenceTokens retires every live preference token the way a real
// revocation does, through the column the resolver reads.
func revokePreferenceTokens(t *testing.T, c *consentEnv) {
	t.Helper()
	if _, err := c.Owner.Exec(context.Background(),
		`UPDATE preference_token SET revoked_at = now() WHERE revoked_at IS NULL`); err != nil {
		t.Fatalf("revoking the preference tokens: %v", err)
	}
}

// freshExpiredConfirmToken mints a link and ages it past its TTL in place. The
// token itself is real and server-issued; only its clock is moved.
func freshExpiredConfirmToken(t *testing.T, c *consentEnv) string {
	t.Helper()
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent/confirm-request",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("minting a link to expire → %d", status)
	}
	token := confirmLinkToken(t, c.AppEnv)
	if _, err := c.Owner.Exec(context.Background(),
		`UPDATE confirm_token SET expires_at = now() - interval '1 day' WHERE consumed_at IS NULL`); err != nil {
		t.Fatalf("ageing the confirm link past its TTL: %v", err)
	}
	return token
}
