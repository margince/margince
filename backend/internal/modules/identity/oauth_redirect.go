// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Where an authorization may be sent back to, and how. There are three rules.
// redirectURIFault says what a client may register. redirectURIMatches says
// what a registered URI matches at authorize time. redirectToClient delivers a
// decision to the address that passed both. The first two separate a
// working native client from an open redirect. oauth_redirect_test.go is this
// file's suite.

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// A redirect URI's query must survive delivery verbatim, so malformed escapes
// are refused at registration while the client can still fix them.
func redirectURIFault(raw string) string {
	const schemeRule = "a redirect URI must be https, or http on localhost"
	u, err := url.Parse(raw)
	switch {
	case err != nil:
		if _, malformedEscape := errors.AsType[url.EscapeError](err); malformedEscape {
			return "a redirect URI must use valid percent-escapes"
		}
		return "a redirect URI must be an absolute URL"
	case u.Fragment != "":
		return "a redirect URI must not carry a fragment"
	}
	if _, err := url.ParseQuery(u.RawQuery); err != nil {
		return "a redirect URI's query must be well formed"
	}
	switch {
	case u.Scheme == "https" && u.Host != "":
		return ""
	case u.Scheme == "http" && isLoopbackHost(u.Hostname()):
		return ""
	default:
		return schemeRule
	}
}

// redirectURIMatches compares a registered redirect URI with a presented one.
// Non-loopback URIs must match exactly. Loopback URIs match ignoring the PORT
// (RFC 8252 §7.3): a native client binds an ephemeral port per session, so an
// exact comparison refuses every CLI client — Claude Code, Cursor, MCP
// Inspector and mcp-remote all behave this way.
func redirectURIMatches(registered, presented string) bool {
	if registered == presented {
		return true
	}
	reg, err := url.Parse(registered)
	if err != nil {
		return false
	}
	pres, err := url.Parse(presented)
	if err != nil {
		return false
	}
	if !isLoopbackHost(reg.Hostname()) || !isLoopbackHost(pres.Hostname()) {
		return false
	}
	return reg.Scheme == pres.Scheme && reg.Hostname() == pres.Hostname() && reg.Path == pres.Path
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// redirectToClient answers the CLIENT at its own redirect_uri. The code on
// approval and access_denied on refusal both come through here, so neither can
// forget to echo state. Each names this server as issuer (RFC 9207), for a
// client that talks to two authorization servers.
func (h Handlers) redirectToClient(w http.ResponseWriter, r *http.Request, req authorizeRequest, answer url.Values) {
	if issuer := h.issuer(); issuer != "" {
		answer.Set(oauthParamIssuer, issuer)
	}
	location, err := clientResponseURI(req, answer)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	// Not an open redirect: the target was matched against the client's
	// registered redirect_uris in validateAuthorize; an unregistered URI never
	// reaches this line.
	http.Redirect(w, r, location, http.StatusFound) // #nosec G710
}

// responseParams is every parameter an authorization RESPONSE is made of (RFC
// 6749 §4.1.2 and §4.1.2.1). Each is deleted from the redirect_uri's own query
// before the answer is applied, so all of them in the delivered Location come
// from this server and none from the URI the client presented.
//
// Success and error are DISJOINT responses in the RFC, and a client reads
// whichever it looks for first: a `code` sitting beside an `error` — or a
// `state` the client never sent — makes it act on an answer nobody gave. This
// is not merely a registration mistake either. redirectURIMatches compares
// scheme, host and path only, so for a loopback client the presented URI's
// query is never validated and a preset response parameter is reachable by
// whoever composes the authorize request.
var responseParams = []string{"code", oauthParamError, "error_description", "error_uri", oauthParamState, oauthParamIssuer}

// clientResponseURI is the absolute Location one authorization response is
// delivered at: the client's redirect_uri carrying our answer, its own query
// otherwise preserved, and nothing of its own that could be mistaken for part
// of the answer.
func clientResponseURI(req authorizeRequest, answer url.Values) (string, error) {
	location, err := url.Parse(req.RedirectURI)
	if err != nil {
		// validateAuthorize matched this against a redirect_uri that already
		// parsed at registration, so an unparseable one here is this server
		// contradicting itself rather than anything the caller sent.
		return "", fmt.Errorf("oauth: redirect_uri does not parse: %w", err)
	}
	// A query we cannot parse is refused, not delivered. Dropping the pair that
	// failed to decode would hand the client a callback URL it did not register.
	// redirectURIFault refuses such a registration; this is the door behind it.
	params, err := url.ParseQuery(location.RawQuery)
	if err != nil {
		return "", fmt.Errorf("oauth: redirect_uri query does not parse: %w", err)
	}
	for _, name := range responseParams {
		params.Del(name)
	}
	maps.Copy(params, answer)
	// An absent state means NO state parameter, not an empty one: a client that
	// sent none must not be handed one to compare against.
	if req.State != "" {
		params.Set(oauthParamState, req.State)
	}
	location.RawQuery = params.Encode()
	// redirectURIFault refuses a fragment at registration. The same rule holds
	// at delivery, so a fragment added to a presented loopback URI stays out of
	// the Location. URL.String() emits a fragment only when Fragment is set.
	location.Fragment = ""
	return location.String(), nil
}
