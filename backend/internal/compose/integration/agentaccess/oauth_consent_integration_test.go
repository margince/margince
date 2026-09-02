// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// The consent screen's read model (GET /oauth/consent-request): the fixed
// scope vocabulary every client is offered (consentRequestPayload —
// unit-tested at identity's TestConsentPayloadOffersTheWholeVocabulary), and
// the deployment switch it follows like every other /oauth/ path. The old
// per-human passport-selection read model this endpoint used to serve is gone
// along with the flow it supported.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// consentCookieName is the double-submit cookie the authorize GET arms. Spelled
// here rather than imported: identity keeps it unexported, and a test that
// restates the wire name catches a rename that would silently break every
// browser mid-flow.
const consentCookieName = "crm_oauth_consent"

// registerClientDirectly inserts a live oauth_client row over the owner
// connection. The harness's normal path to a live client is POST
// /oauth/register, but that endpoint is itself part of the connector's
// gated route group — unavailable in exactly the deployment state (connector
// off) a test needs a live client to probe.
func registerClientDirectly(t *testing.T, e *apptest.AppEnv, clientID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := e.Owner.Exec(ctx,
		`INSERT INTO oauth_client (client_id, client_name, redirect_uris)
		 VALUES ($1, 'directly registered', ARRAY['https://client.example/cb']::text[])`, clientID); err != nil {
		t.Fatalf("inserting a live oauth_client row: %v", err)
	}
}

// This read follows the connector's deployment switch exactly like every
// other /oauth/ path: a signed-in human asking about a client that
// genuinely exists gets the real answer only while the connector is
// declared, and the identical apperrors.ErrNotFound every absent /oauth/
// path answers once it is not. Both halves probe the SAME client id,
// inserted directly rather than through /oauth/register — that endpoint is
// itself ungated only while the connector is on, so it cannot supply the
// fixture for the off case — which keeps client existence constant and
// leaves the connector switch as the only variable between them.
func TestConsentRequestFollowsTheConnectorSwitch(t *testing.T) {
	const clientID = "directly-registered-client"
	// A request the CONTRACT cannot bind is refused by the generated router
	// before any middleware or handler runs, so the switch has no say in it.
	// That refusal is captured in both states and compared below: it is the
	// one answer this operation gives that the gate cannot change, and the
	// thing an off switch must hide is the DEPLOYMENT's state, which two
	// identical refusals disclose nothing about.
	var unbindable [2]int

	t.Run("off", func(t *testing.T) {
		e := apptest.SetupApp(t)
		e.BootstrapWorkspace(t)
		registerClientDirectly(t, e, clientID)

		status := e.Call(t, "GET", "/v1/oauth/consent-request?client_id="+clientID+"&scope=read", nil, nil, nil)
		if status != http.StatusNotFound {
			t.Fatalf("consent-request for a live client, connector off → %d, want 404", status)
		}
		unbindable[0] = e.Call(t, "GET", "/v1/oauth/consent-request?client_id="+clientID, nil, nil, nil)
	})

	t.Run("on", func(t *testing.T) {
		c := setupConnector(t)
		registerClientDirectly(t, c.AppEnv, clientID)

		status := c.Call(t, "GET", "/v1/oauth/consent-request?client_id="+clientID+"&scope=read", nil, nil, nil)
		if status != http.StatusOK {
			t.Fatalf("consent-request for the same live client, connector on → %d, want 200", status)
		}
		unbindable[1] = c.Call(t, "GET", "/v1/oauth/consent-request?client_id="+clientID, nil, nil, nil)
	})

	if unbindable[0] != http.StatusUnprocessableEntity || unbindable[1] != unbindable[0] {
		t.Errorf("a request missing the required scope parameter → %d with the connector off and %d with it on, want %d from both: the parameter refusal must not distinguish the two deployments",
			unbindable[0], unbindable[1], http.StatusUnprocessableEntity)
	}
}

// authorizeNoFollow issues the authorize GET with redirects DISABLED, so the
// 302 itself is the assertion target rather than whatever it points at. The
// Set-Cookie values come back too: the armed nonce is half of the double-submit
// pair the fragment carries the other half of.
func (o *oauthEnv) authorizeNoFollow(t *testing.T, extra url.Values) (int, string, string, []*http.Cookie) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet,
		o.TS.URL+"/oauth/authorize?"+o.authorizeQuery(extra).Encode(), nil)
	if err != nil {
		t.Fatalf("building the authorize request: %v", err)
	}
	// o.client carries the signed-in human's session in its jar and trusts the
	// harness's TLS certificate; a fresh http.Client would have neither.
	o.Client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	defer func() { o.Client.CheckRedirect = nil }()
	resp, err := o.Client.Do(req) //nolint:bodyclose // closed by apptest.CloseBody below; bodyclose only recognises a Close in the same package
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	defer apptest.CloseBody(t, resp)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	return resp.StatusCode, resp.Header.Get("Location"), string(body), resp.Cookies()
}

func cookieValue(t *testing.T, setCookies []*http.Cookie, name string) string {
	t.Helper()
	for _, cookie := range setCookies {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	t.Fatalf("no %s cookie in the response", name)
	return ""
}

// The authorize GET hands the browser to the SPA and mints nothing. The params
// ride in the FRAGMENT, which is never sent to a server — so client_id, state
// and the PKCE challenge stay out of api access logs.
func TestAuthorizeRedirectsToTheSPAConsentScreen(t *testing.T) {
	o := setupOAuth(t)

	status, location, body, setCookies := o.authorizeNoFollow(t, url.Values{"scope": {"read write"}})

	if status != http.StatusFound {
		t.Fatalf("authorize → %d %s, want 302", status, body)
	}
	fragment := "#/oauth-consent?"
	if !strings.Contains(location, fragment) {
		t.Fatalf("Location = %q, want the SPA consent route %q", location, fragment)
	}
	// The old server-rendered page is gone: no HTML, and nothing that could be
	// mistaken for a consent decision.
	if strings.Contains(body, "<form") || strings.Contains(body, "Approve") {
		t.Fatalf("authorize still renders a form: %s", body)
	}
	// Everything the SPA needs must be AFTER the '#', not before it.
	before, after, _ := strings.Cut(location, "#")
	if strings.Contains(before, "client_id") {
		t.Fatalf("client_id leaked into the server-visible part of %q", location)
	}
	for _, param := range []string{"client_id", "state", "code_challenge", "redirect_uri", "scope", "consent"} {
		if !strings.Contains(after, param) {
			t.Fatalf("fragment %q is missing %s, which the SPA must POST back", after, param)
		}
	}
	// The consent nonce travels in the fragment and matches the cookie the same
	// response armed — the SPA cannot read that HttpOnly cookie, and the cookie
	// is Path=/oauth/authorize so no other endpoint can echo it either. The POST
	// then proves possession of BOTH.
	fragmentParams, err := url.ParseQuery(after[strings.Index(after, "?")+1:])
	if err != nil {
		t.Fatalf("parsing the fragment query %q: %v", after, err)
	}
	nonce := fragmentParams.Get("consent")
	if nonce == "" {
		t.Fatal("the fragment carries no consent nonce, so the POST can never satisfy the double-submit check")
	}
	if got := cookieValue(t, setCookies, consentCookieName); got != nonce {
		t.Fatalf("cookie %s = %q, want the fragment's nonce %q", consentCookieName, got, nonce)
	}
	// The nonce must not be visible to the server on the redirect it rode in on.
	if strings.Contains(before, nonce) {
		t.Fatalf("the consent nonce leaked into the server-visible part of %q", location)
	}
}

// The handshake an empty account can finish: `claude mcp add` for a brand-new
// human who has never minted a passport by hand. Before the scopes-checkbox
// flow replaced lending, the consent screen offered no approve control at
// all without one — this is the case the whole change exists for, so it is
// held here by a test that fails without it rather than left to be true only
// by construction.

// mcpTools drives tools/list on the connector transport with the given
// bearer and returns the tool entries as untyped objects, so a caller can
// read whichever field it needs (name, description) without this helper
// growing a return type per caller.
//
//craft:ignore naked-any a tools/list entry is an open object by the MCP protocol — asserting on one means reading it untyped
func (o *oauthEnv) mcpTools(t *testing.T, bearer string) []map[string]any {
	t.Helper()
	resp := listTools(t, o.AppEnv, bearer)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tools/list → %d %s", resp.StatusCode, resp.Body)
	}
	result := rpcResult(t, resp.Body)
	raw, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list result carries no tools array: %v", result)
	}
	tools := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		tool, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("tools/list entry is not an object: %v", entry)
		}
		tools = append(tools, tool)
	}
	return tools
}

// toolRequiresScope reads a tool's advertised required scope off the
// description DescribeForClient renders — the same text a human or a model
// reading tools/list sees — rather than re-deriving it from the tool's name,
// which would let this test and the surface it checks drift apart.
func toolRequiresScope(t *testing.T, tool map[string]any, scope string) bool {
	t.Helper()
	description, _ := tool["description"].(string)
	if description == "" {
		t.Fatalf("tool %v carries no description to read its required scope from", tool)
	}
	return strings.Contains(description, fmt.Sprintf("requires passport scope %q", scope))
}

// TestAHumanWithNoPassportCanConnectAClient is the case that has never
// passed: a human with zero passports still authorizes a client end to end,
// on the scopes they tick, with nothing hand-minted first.
func TestAHumanWithNoPassportCanConnectAClient(t *testing.T) {
	o := setupOAuth(t)
	assertOwnerCount(t, o, 0, `SELECT count(*) FROM passport`)

	code := o.authorize(t, url.Values{"scope": {"read draft write send enrich"}})
	status, body := o.exchange(t, url.Values{"code": {code}})
	if status != http.StatusOK {
		t.Fatalf("token → %d %v", status, body)
	}
	token, _ := body["access_token"].(string)
	if token == "" {
		t.Fatal("an account that has minted nothing must still be able to authorize a client")
	}
	if !o.accessTokenWorks(t, token) {
		t.Fatal("the minted connection credential must authenticate")
	}
}

// A connection-minted passport was never lendable, so under the old model
// connecting a second client left the human just as stuck as connecting the
// first — there was nothing of theirs to hand over. Under the scopes model
// there is nothing to run out of: a second client ticks its own scopes and
// connects on the same terms.
func TestASecondClientNeedsNoHandMintedPassportEither(t *testing.T) {
	o := setupOAuth(t)
	code := o.authorize(t, url.Values{"scope": {"read"}})
	if _, body := o.exchange(t, url.Values{"code": {code}}); body["access_token"] == nil || body["access_token"] == "" {
		t.Fatalf("the first client did not connect: %v", body)
	}

	o.clientID = registerSecondClient(t, o)
	code = o.authorize(t, url.Values{"scope": {"read"}})
	status, second := o.exchange(t, url.Values{"code": {code}})
	if token, _ := second["access_token"].(string); status != http.StatusOK || token == "" {
		t.Fatalf("the second client must connect on the same terms as the first: %d %v", status, second)
	}
}

// The tool list is the proof a human can check: a connection granted only
// read must not be offered a write tool. Asserting the passport's scope
// column instead would prove the row, not the surface a client actually
// sees, and an empty list would pass every assertion below vacuously.
func TestNarrowingOnTheScreenReachesTheToolSurface(t *testing.T) {
	o := setupOAuth(t)
	code := o.authorize(t, url.Values{"scope": {"read"}})
	_, body := o.exchange(t, url.Values{"code": {code}})
	token, _ := body["access_token"].(string)
	if token == "" {
		t.Fatalf("handshake left the client without a credential: %v", body)
	}

	tools := o.mcpTools(t, token)
	if len(tools) == 0 {
		t.Fatal("a read connection listed no tools at all; an empty list would pass this test vacuously")
	}
	sawReadTool := false
	for _, tool := range tools {
		for _, excluded := range []string{"draft", "write", "send", "enrich"} {
			if toolRequiresScope(t, tool, excluded) {
				name, _ := tool["name"].(string)
				t.Fatalf("a read-only connection was offered the %s-scoped tool %q", excluded, name)
			}
		}
		if toolRequiresScope(t, tool, "read") {
			sawReadTool = true
		}
	}
	if !sawReadTool {
		t.Fatal("a read connection listed no read-scoped tool; the surface should narrow, not vanish")
	}
}
