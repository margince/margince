// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/capture/graph"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const testStateKey = "a-32-byte-or-longer-signing-key!!"

// wiredHandlers builds connectorHandlers with a real signer + real (pure)
// Google OAuth clients (gmail + gcal) and a non-nil registry, so the non-DB
// paths (connect URL, state verification, provider gating) exercise real code.
// The registry's DB methods are never reached on these paths.
func wiredHandlers() connectorHandlers {
	return connectorHandlers{
		registry:  capture.NewRegistry(nil, nil, liveAuthority{}, nil),
		authority: liveAuthority{},
		// Built through the same newGmailOAuth the production wiring uses, so
		// a change to the scopes it requests is exercised here too — not
		// duplicated as a second, driftable literal.
		oauth:         newGmailOAuth(GmailConfig{ClientID: "cid", ClientSecret: "sec"}),
		gmailAPI:      gmail.NewAPI(nil, ""),
		gcalOAuth:     gcal.NewOAuth(gcal.OAuthConfig{ClientID: "cid", ClientSecret: "sec"}),
		gcalAPI:       gcal.NewAPI(nil, ""),
		signer:        newStateSigner([]byte(testStateKey)),
		publicBaseURL: "https://app.test", // the SPA/front origin — landing
		apiBaseURL:    "https://api.test", // the api origin — callback redirect_uri
	}
}

// gmailAuthCodeURLForTest drives the real ConnectConnector handler for gmail
// and returns the decoded `scope` query parameter of the resulting consent
// URL, so a test can assert on exactly what Google will be asked to grant.
func gmailAuthCodeURLForTest(t *testing.T) string {
	t.Helper()
	h := wiredHandlers()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/gmail/connect", nil).WithContext(humanCtx())

	h.ConnectConnector(rec, req, "gmail")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	var resp crmcontracts.ConnectConnectorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AuthorizeUrl == nil {
		t.Fatal("authorize_url missing")
	}
	u, err := url.Parse(*resp.AuthorizeUrl)
	if err != nil {
		t.Fatalf("authorize_url not a URL: %v", err)
	}
	return u.Query().Get("scope")
}

func humanCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.MustParse("11111111-1111-1111-1111-111111111111"))
	return principal.WithActor(ctx, principal.Principal{
		Type:   principal.PrincipalHuman,
		ID:     "human:22222222-2222-2222-2222-222222222222",
		UserID: ids.MustParse("22222222-2222-2222-2222-222222222222"),
		Scopes: principal.NewScopeSet(principal.ScopeRead),
	})
}

func TestConnectConnectorReturnsSignedAuthorizeURL(t *testing.T) {
	h := wiredHandlers()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/gmail/connect", nil).WithContext(humanCtx())

	h.ConnectConnector(rec, req, "gmail")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	var resp crmcontracts.ConnectConnectorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AuthorizeUrl == nil {
		t.Fatal("authorize_url missing")
	}
	u, err := url.Parse(*resp.AuthorizeUrl)
	if err != nil {
		t.Fatalf("authorize_url not a URL: %v", err)
	}
	// The redirect_uri points back at our callback, and the state must verify.
	if got := u.Query().Get("redirect_uri"); got != "https://api.test/v1/connectors/gmail/callback" {
		t.Errorf("redirect_uri = %q, want the api callback", got)
	}
	st, err := h.signer.verify(u.Query().Get("state"), time.Now())
	if err != nil {
		t.Fatalf("authorize_url state does not verify: %v", err)
	}
	if st.Provider != "gmail" || st.User != ids.MustParse("22222222-2222-2222-2222-222222222222") {
		t.Errorf("state = %+v, want gmail + the acting user", st)
	}
}

// Google will not add a scope to an existing refresh token, so both permissions
// must ride ONE consent — otherwise sending needs a second connection.
func TestGmailConsentRequestsReadAndSendScopes(t *testing.T) {
	got := gmailAuthCodeURLForTest(t)
	for _, want := range []string{
		"gmail.readonly",
		"gmail.send",
	} {
		if !strings.Contains(got, want) && !strings.Contains(got, url.QueryEscape("https://www.googleapis.com/auth/"+want)) {
			t.Errorf("consent URL missing scope %q\nurl: %s", want, got)
		}
	}
}

func TestConnectConnectorReturnsSignedAuthorizeURLForGcal(t *testing.T) {
	h := wiredHandlers()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/gcal/connect", nil).WithContext(humanCtx())

	h.ConnectConnector(rec, req, "gcal")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for gcal (body %s)", rec.Code, rec.Body)
	}
	var resp crmcontracts.ConnectConnectorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AuthorizeUrl == nil {
		t.Fatal("authorize_url missing")
	}
	u, err := url.Parse(*resp.AuthorizeUrl)
	if err != nil {
		t.Fatalf("authorize_url not a URL: %v", err)
	}
	// The redirect_uri and the signed state must both be gcal's.
	if got := u.Query().Get("redirect_uri"); got != "https://api.test/v1/connectors/gcal/callback" {
		t.Errorf("redirect_uri = %q, want the gcal callback", got)
	}
	// The calendar consent scope must be requested — and ONLY it, never the mail
	// scope (least privilege: a calendar connect must not silently grant mail).
	if got := u.Query().Get("scope"); !strings.Contains(got, "calendar.readonly") {
		t.Errorf("scope = %q, want the calendar.readonly consent", got)
	} else if strings.Contains(got, "gmail.readonly") {
		t.Errorf("scope = %q, must not request the gmail.readonly scope", got)
	}
	// gcal authorizes separately from Gmail: no incremental authorization, so
	// the calendar credential can't accrete the mail-read grant.
	if u.Query().Get("include_granted_scopes") != "" {
		t.Errorf("gcal consent must not send include_granted_scopes (per-connector least privilege)")
	}
	st, err := h.signer.verify(u.Query().Get("state"), time.Now())
	if err != nil {
		t.Fatalf("authorize_url state does not verify: %v", err)
	}
	if st.Provider != "gcal" {
		t.Errorf("state provider = %q, want gcal", st.Provider)
	}
}

func TestConnectConnectorRejectsUnsupportedProvider(t *testing.T) {
	h := wiredHandlers()
	rec := httptest.NewRecorder()
	// A provider this OAuth transport does not handle (not gmail/gcal/graph, and
	// not the separate imap surface) must be refused with a clean
	// connector_unsupported, never misrouted onto another provider's flow.
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/whatsapp/connect", nil).WithContext(humanCtx())

	h.ConnectConnector(rec, req, "whatsapp")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for an unsupported provider", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "connector_unsupported") {
		t.Errorf("body should carry connector_unsupported: %s", rec.Body)
	}
}

func TestConnectConnectorNotImplementedWhenUnwired(t *testing.T) {
	var h connectorHandlers // zero value: no oauth/registry
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/gmail/connect", nil).WithContext(humanCtx())

	h.ConnectConnector(rec, req, "gmail")

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501 when the Gmail app is not wired", rec.Code)
	}
}

func TestCallbackDeniedRedirectsHonestly(t *testing.T) {
	h := wiredHandlers()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/gmail/callback", nil)
	denied := "access_denied"

	h.ConnectorOAuthCallback(rec, req, "gmail", crmcontracts.ConnectorOAuthCallbackParams{Error: &denied})

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://app.test/#/onboarding/connect/denied/gmail" {
		t.Errorf("Location = %q, want the denied landing", loc)
	}
}

func TestCallbackBadStateRedirectsError(t *testing.T) {
	h := wiredHandlers()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/gmail/callback", nil)
	code := "the-code"

	// A forged/garbage state must not proceed to a token exchange.
	h.ConnectorOAuthCallback(rec, req, "gmail", crmcontracts.ConnectorOAuthCallbackParams{
		Code:  &code,
		State: "not-a-valid-signed-state",
	})

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://app.test/#/onboarding/connect/error/gmail" {
		t.Errorf("Location = %q, want the error landing", loc)
	}
}

func TestLandingURLMapsReturnToThroughAClosedSet(t *testing.T) {
	h := connectorHandlers{publicBaseURL: "https://crm.example.com/"}
	for _, tc := range []struct {
		name     string
		returnTo string
		want     string
	}{
		{"settings", "settings", "https://crm.example.com/#/settings/connections/ok/graph"},
		{"onboarding", "onboarding", "https://crm.example.com/#/onboarding/connect/ok/graph"},
		{"absent falls back to onboarding", "", "https://crm.example.com/#/onboarding/connect/ok/graph"},
		{"unknown falls back to onboarding", "elsewhere", "https://crm.example.com/#/onboarding/connect/ok/graph"},
		{"a URL is never reflected", "https://evil.example/", "https://crm.example.com/#/onboarding/connect/ok/graph"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.landingURL("ok", tc.returnTo, "graph"); got != tc.want {
				t.Errorf("landingURL(ok, %q, graph) = %q, want %q", tc.returnTo, got, tc.want)
			}
		})
	}
}

// The landing surface has to know WHICH mailbox this round-trip connected: a
// workspace with both Gmail and Microsoft live would otherwise offer the
// import for whichever one the roster lists first.
func TestLandingURLNamesTheProviderTheConsentReturnedFor(t *testing.T) {
	h := connectorHandlers{publicBaseURL: "https://crm.example.com"}
	for _, provider := range []string{"gmail", "graph", "gcal"} {
		want := "https://crm.example.com/#/onboarding/connect/ok/" + provider
		if got := h.landingURL(outcomeOK, returnToOnboarding, provider); got != want {
			t.Errorf("landingURL(ok, onboarding, %q) = %q, want %q", provider, got, want)
		}
	}
}

// What the human reads has to match what they can do. Three failures that all
// used to render "please try again": one where trying again is right, one where
// it repeats the same refusal, and one that no user action can ever clear.
func TestConnectFailureOutcomeMatchesTheRemedy(t *testing.T) {
	disabledAPI := &connector.ProviderError{
		Op: "/calendars/primary", Status: http.StatusForbidden,
		Reason: "accessNotConfigured", Class: gcal.ErrAuthRejected,
	}
	revokedGrant := &connector.ProviderError{
		Op: "token", Status: http.StatusBadRequest,
		Reason: "invalid_grant", Class: gcal.ErrAuthRejected,
	}
	unreachable := &connector.ProviderError{
		Op: "token", Status: http.StatusBadGateway, Class: gcal.ErrUnreachable,
	}

	// A refused OAuth client is the deployment's own credentials, not the
	// human's grant — and unlike the disabled API it is provider-independent.
	badClient := &connector.ProviderError{
		Op: "token", Status: http.StatusUnauthorized,
		Reason: "invalid_client", Class: gcal.ErrAuthRejected,
	}

	for _, tc := range []struct {
		name     string
		provider string
		err      error
		want     string
	}{
		{"a disabled provider API needs an administrator", providerGcal, disabledAPI, outcomeMisconfigured},
		// SEPARATE from the disabled API, because the remedies are separate
		// screens: that one is the vendor's console, this one is the app card in
		// Settings. Folded together, a Microsoft installation — which can only
		// ever reach this branch — read every credential mistake as an unenabled
		// API it does not have.
		{"a refused OAuth client sends its administrator to the app card", providerGcal, badClient, outcomeBadClient},
		{"a refused OAuth client is provider-independent", providerGraph, badClient, outcomeBadClient},
		{"a refused grant needs its human", providerGcal, revokedGrant, outcomeRejected},
		{"an unreachable provider is worth retrying", providerGcal, unreachable, outcomeError},
		{"a bare auth sentinel carries no remedy detail", providerGcal, gcal.ErrAuthRejected, outcomeRejected},
		{"an unattributed failure stays generic", providerGcal, errors.New("something of ours broke"), outcomeError},
		// Google's reason vocabulary must not be read against Microsoft: the
		// remedy names a Google Cloud project that has nothing to do with it.
		{"a Google reason code is not consulted for graph", providerGraph, disabledAPI, outcomeRejected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := connectFailureOutcome(tc.provider, tc.err); got != tc.want {
				t.Errorf("connectFailureOutcome(%s, %v) = %q, want %q", tc.provider, tc.err, got, tc.want)
			}
		})
	}
}

// A human who declines consent from Settings has to land back in Settings — on
// the onboarding screen they never see the note explaining what happened. The
// signed state is what names the surface, so it is verified before the outcome
// is branched on.
func TestCallbackDenialReturnsToTheSurfaceItStartedFrom(t *testing.T) {
	h := wiredHandlers()
	denied := "access_denied"
	state := h.signer.sign(connectState{
		Workspace: ids.MustParse("11111111-1111-1111-1111-111111111111"),
		User:      ids.MustParse("22222222-2222-2222-2222-222222222222"),
		Provider:  "gmail",
		Nonce:     "a-nonce",
		ReturnTo:  returnToSettings,
	}, time.Now().Add(time.Minute))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/gmail/callback", nil)
	h.ConnectorOAuthCallback(rec, req, "gmail", crmcontracts.ConnectorOAuthCallbackParams{
		Error: &denied,
		State: state,
	})

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://app.test/#/settings/connections/denied/gmail" {
		t.Errorf("Location = %q, want the denial to land back in Settings", loc)
	}
}

// The mirror of the above: a denial whose state cannot be trusted has no
// trustworthy ReturnTo either, so it keeps the default rather than honoring a
// surface an attacker chose.
func TestCallbackDenialWithUntrustedStateKeepsTheDefaultSurface(t *testing.T) {
	h := wiredHandlers()
	denied := "access_denied"

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/gmail/callback", nil)
	h.ConnectorOAuthCallback(rec, req, "gmail", crmcontracts.ConnectorOAuthCallbackParams{
		Error: &denied,
		State: "forged",
	})

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://app.test/#/onboarding/connect/denied/gmail" {
		t.Errorf("Location = %q, want the onboarding default", loc)
	}
}

// A provider this installation never registered an app for must agree between
// the two surfaces that both answer the question: the roster reports
// app_missing, and clicking connect for the very same provider still answers
// the declared 501, never a click that surprises a screen that just said
// "connect would proceed" and never one that contradicts what it just listed.
func TestConnectabilityAgreesWithConnectConnectorWhenNoAppIsRegistered(t *testing.T) {
	h := wiredHandlers() // gmail + gcal wired; graph is not.

	avail := h.connectability(context.Background(), "graph")
	if avail.reason != connectAppMissing {
		t.Fatalf("connectability(graph) = %+v, want app_missing", avail)
	}
	if avail.err != nil {
		t.Errorf("connectability(graph).err = %v, want nil for an unregistered app", avail.err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/graph/connect", nil).WithContext(humanCtx())
	h.ConnectConnector(rec, req, "graph")

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("ConnectConnector(graph) status = %d, want 501 to match the app_missing roster entry", rec.Code)
	}
}

// A stored app that will not resolve reports app_unusable on its own roster
// entry, and, the point of reporting per-provider rather than failing the
// whole read, every OTHER provider still appears rather than the one bad
// app taking the roster down with it.
func TestConnectabilityReportsAppUnusableWithoutFailingTheOtherProviders(t *testing.T) {
	h := wiredHandlers()
	// A non-nil transport clears the early gate in graphApp so it reaches the
	// credential read; the resolver below is what fails.
	h.graphOAuth = graph.NewOAuth(graph.OAuthConfig{ClientID: "cid", ClientSecret: "sec"})
	boom := errors.New("vault: sealed secret will not open")
	h.microsoftCredentials = func(context.Context) (capture.ConnectorApp, bool, error) {
		return capture.ConnectorApp{}, false, boom
	}

	avail := h.connectability(context.Background(), "graph")
	if avail.reason != connectAppUnusable {
		t.Fatalf("connectability(graph) = %+v, want app_unusable", avail)
	}
	if !errors.Is(avail.err, boom) {
		t.Errorf("connectability(graph).err = %v, want the resolution failure", avail.err)
	}

	// The exact loop ListConnectors runs to build the roster (its own DB read
	// for the connection list needs a live pool, which this unit test has
	// none of; the invariant under test is this loop, not that read).
	got := map[string]connectResult{}
	for _, p := range listedProviders {
		got[p] = h.connectability(context.Background(), p).reason
	}
	if got[providerGraph] != connectAppUnusable {
		t.Errorf("graph reason = %q, want app_unusable", got[providerGraph])
	}
	if got[providerGmail] != connectReady {
		t.Errorf("gmail reason = %q, want ready, graph's failure must not reach it", got[providerGmail])
	}
	if got[providerIMAP] != connectReady {
		t.Errorf("imap reason = %q, want ready, graph's failure must not reach it", got[providerIMAP])
	}
}

// IMAP never needs an OAuth app: its credentials are per-connection and
// vault-sealed, so the registry alone decides whether it is ready.
func TestConnectabilityImapIsReadyWithJustTheRegistry(t *testing.T) {
	h := wiredHandlers()

	if avail := h.connectability(context.Background(), "imap"); avail.reason != connectReady {
		t.Errorf("connectability(imap) = %+v, want ready", avail)
	}

	var unwired connectorHandlers // zero value: no registry
	if avail := unwired.connectability(context.Background(), "imap"); avail.reason != connectUnsupported {
		t.Errorf("connectability(imap) with no registry = %+v, want unsupported", avail)
	}
}

// AN INSTALLATION WITH NO APP IS TOLD WHICH APP IS MISSING, not that the
// feature does not exist.
//
// 501 is right for both "nobody built this route" and "this deployment has not
// configured it", and only the first is what the generic stub text describes.
// Now that an app can be supplied in Settings, the second is a state a working
// installation passes through on its way in — and the generic sentence sends an
// operator looking for a newer build instead of to the screen that fixes it.
func TestConnectingWithNoStoredAppSaysWhichAppIsMissing(t *testing.T) {
	// A WIRED deployment with no app for THIS vendor: the registry exists
	// (gmail is composed), and graph has nothing stored.
	h := wiredHandlers()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/graph/connect", nil).WithContext(humanCtx())

	h.ConnectConnector(rec, req, "graph")

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501 — the declared answer for an unconfigured connector", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "not yet implemented") {
		t.Errorf("the body still reads as an unbuilt feature: %s", body)
	}
	for _, want := range []string{"graph", "Settings"} {
		if !strings.Contains(body, want) {
			t.Errorf("the body does not mention %q, so it does not say what to go do: %s", want, body)
		}
	}
}

// A deployment that wired NO capture keeps the generic sentence. Sending its
// operator to the app card would be sending them somewhere that cannot make the
// route work — there is no registry for an app to be resolved against.
func TestConnectingWithNoCaptureAtAllStaysGeneric(t *testing.T) {
	h := connectorHandlers{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/graph/connect", nil).WithContext(humanCtx())

	h.ConnectConnector(rec, req, "graph")

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "Settings") {
		t.Errorf("a deployment with no capture registry was told to visit Settings: %s", body)
	}
}
