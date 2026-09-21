// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The address a buyer invitation MAILS is an address this app serves.
//
// The link is minted by the server and opened by a browser, so the route and
// its query parameter are one invariant spelled on both sides of a wire. Left
// unheld, they drift — and the drift is silent in the only direction that
// matters, because the seller sees an invitation sent and the buyer sees a 404.
//
// This is not hypothetical. The mailer was written first and `cmd/api` refused
// to wire it precisely because `room` was not a route the SPA served: the whole
// feature sat behind a link that pointed at the not-found page, and the recipient
// would have spent the one credential they were issued getting there.
//
// A buyer credential is single-use, which is what makes a wrong link worse than
// a broken one: following it consumes the invitation. There is no second click.
//
// Both directions, and all four facts: the screen the link names is a screen
// this app has, it is reachable without a session, it draws no rail, and the
// query key the mailer writes is the key the router reads.

import (
	"regexp"
	"strings"
	"testing"
)

const (
	inviteMailSource = "internal/modules/dealrooms/invitemail.go"
	routerSource     = "../frontend/src/app/router.tsx"
	navSource        = "../frontend/src/app/nav.ts"
	// PUBLIC_SCREENS lives with the shell that enforces it rather than with the
	// route table, because it is a question about the SESSION and not about the
	// address.
	shellSource = "../frontend/src/App.tsx"
)

// buyerRouteConst reads the server's own spelling of the link: the fragment
// route and the query key the credential rides in.
var buyerRouteConst = regexp.MustCompile(`buyerRoute\s*=\s*"/#/([a-z-]+)\?([a-z_]+)="`)

// hashCredentialEntry reads one `{ screen: "x", param: "y" }` pair out of the
// router's credential registry.
var hashCredentialEntry = regexp.MustCompile(`\{\s*screen:\s*"([a-z-]+)",\s*param:\s*"([a-z_]+)"\s*\}`)

func TestTheBuyerInviteLinkNamesARouteTheAppServes(t *testing.T) {
	t.Parallel()

	route, param := serverBuyerRoute(t)
	router := readSource(t, routerSource)

	// SCREENS is what parseHash admits; anything else answers not-found, which
	// is the 404 this gate exists to keep a buyer out of.
	if !listedInArray(t, router, "SCREENS", route) {
		t.Errorf("the invitation mails /#/%s, which the SPA's SCREENS does not list — parseHash answers not-found for an unlisted first segment, so the buyer lands on the 404 page having spent the single-use credential that took them there",
			route)
	}
	// A buyer holds no CRM session. A route outside PUBLIC_SCREENS is one the
	// app sends to sign-in, and there is no account behind the address the
	// invitation went to.
	if !listedInArray(t, readSource(t, shellSource), "PUBLIC_SCREENS", route) {
		t.Errorf("the invitation mails /#/%s, which is not in PUBLIC_SCREENS — a buyer has no session to be recognised by, so the app would send them to sign in for an account that does not exist",
			route)
	}
	// The rail is the seller's chrome. Drawing it around a buyer would offer
	// navigation into a workspace the room deliberately does not admit them to.
	if !listedInArray(t, readSource(t, navSource), "RAIL_LESS_SCREENS", route) {
		t.Errorf("the invitation mails /#/%s, which is not in RAIL_LESS_SCREENS — the buyer would be given the seller's own chrome around a room that admits them to nothing else",
			route)
	}

	registered, found := "", false
	for _, m := range hashCredentialEntry.FindAllStringSubmatch(router, -1) {
		if m[1] == route {
			registered, found = m[2], true
		}
	}
	if !found {
		t.Fatalf("the router registers no credential parameter for %q, so the screen is served and the credential in the link is never read off it — the buyer arrives at an empty room and the invitation is spent",
			route)
	}
	if registered != param {
		t.Errorf("the invitation writes ?%s= and the router reads ?%s= — the link resolves to the screen and hands it nothing, which is the one failure that looks like the feature working",
			param, registered)
	}
}

// serverBuyerRoute reads the route and query key out of the mailer's own
// constant, so this compares the string that is actually sent rather than one
// restated here.
func serverBuyerRoute(t *testing.T) (route, param string) {
	t.Helper()
	m := buyerRouteConst.FindStringSubmatch(readSource(t, inviteMailSource))
	if m == nil {
		t.Fatalf("%s declares no buyerRoute of the shape \"/#/<route>?<param>=\" — this gate reads the link off that constant, and a link built some other way is one it cannot check at all",
			inviteMailSource)
	}
	return m[1], m[2]
}

// listedInArray reports whether a name appears inside the named declaration's
// bracketed body. Scoped to the declaration rather than searched for anywhere in
// the file, because `room` appears in this router several times over and a
// file-wide match would agree with a screen that was never listed.
func listedInArray(t *testing.T, source, declaration, name string) bool {
	t.Helper()
	at := strings.Index(source, declaration)
	if at < 0 {
		t.Fatalf("the frontend declares no %s — the mirror this gate compares against is gone, and an absent list agrees with everything", declaration)
	}
	open := strings.Index(source[at:], "[")
	closed := strings.Index(source[at:], "]")
	if open < 0 || closed < open {
		t.Fatalf("the frontend's %s is not a bracketed list, so this gate cannot tell what it holds", declaration)
	}
	return strings.Contains(source[at+open:at+closed], `"`+name+`"`)
}
