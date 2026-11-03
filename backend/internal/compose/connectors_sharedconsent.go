// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One Google screen for mail and calendar. A Gmail consent that also_connects
// the calendar asks for the calendar's scopes too, and the callback seals the
// one grant into both connections. Microsoft keeps its chained second consent.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
)

// sharedConsentCalendars are the calendars a mail consent can grant on its own
// screen. Each names the scopes it adds, and the one scope it needs to sync.
var sharedConsentCalendars = map[string]struct {
	scopes   func() []string
	requires string
}{
	providerGcal: {scopes: gcal.ConsentScopes, requires: gcal.ReadScope},
}

// consentCode is the authorization code one consent returned. Google's code is
// single use, so the first Exchange keeps the grant. A second connector sealing
// the same consent is answered with that grant.
type consentCode struct {
	code        string
	redirectURI string
	grant       *oauthflow.TokenGrant
}

func newConsentCode(code, redirectURI string) *consentCode {
	return &consentCode{code: code, redirectURI: redirectURI}
}

// googleAuthorizer wraps a Google connector's own token client so that its
// Exchange redeems this code at most once.
//
//nolint:ireturn // the connector constructors take the Authorizer seam
func (c *consentCode) googleAuthorizer(inner googleconn.Authorizer) googleconn.Authorizer {
	return redeemOnce{Authorizer: inner, consent: c}
}

// granted reports whether the redeemed grant carries scope. The user can untick
// a scope on Google's screen, and the token response then leaves it out.
func (c *consentCode) granted(scope string) bool {
	return c.grant != nil && slices.Contains(c.grant.Scopes, scope)
}

type redeemOnce struct {
	googleconn.Authorizer
	consent *consentCode
}

func (r redeemOnce) Exchange(ctx context.Context, code, redirectURI string) (oauthflow.TokenGrant, error) {
	if r.consent.grant != nil {
		return *r.consent.grant, nil
	}
	grant, err := r.Authorizer.Exchange(ctx, code, redirectURI)
	if err != nil {
		return oauthflow.TokenGrant{}, err
	}
	r.consent.grant = &grant
	return grant, nil
}

// sharesConsent reports whether a mail consent chained to calendar asks for
// both on one screen.
func sharesConsent(mail, calendar string) bool {
	_, ok := sharedConsentCalendars[calendar]
	return ok && pairedCalendar[mail] == calendar
}

// withCalendarScopes adds the calendar's scopes to a mail consent URL. The mail
// app built the URL, so its own scopes, redirect and state stay as they are.
func withCalendarScopes(consentURL, calendar string) (string, error) {
	u, err := url.Parse(consentURL)
	if err != nil {
		return "", fmt.Errorf("shared consent: reading the mail consent URL: %w", err)
	}
	q := u.Query()
	scopes := strings.Fields(q.Get("scope"))
	for _, scope := range sharedConsentCalendars[calendar].scopes() {
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	q.Set("scope", strings.Join(scopes, " "))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// connectSharedCalendar connects the calendar from the grant the mail leg just
// stored, and lands on the calendar's outcome as a chained consent would. An
// unticked calendar lands as declined, and the mailbox stays connected.
func (h connectorHandlers) connectSharedCalendar(ctx context.Context, w http.ResponseWriter, r *http.Request, st connectState, held int, consent *consentCode) {
	land := func(outcome string) {
		http.Redirect(w, r, h.landingURL(outcome, st.ReturnTo, st.Then, held), http.StatusFound)
	}
	if consent == nil || !consent.granted(sharedConsentCalendars[st.Then].requires) {
		land(outcomeDenied)
		return
	}
	app, ok, err := h.oauthApp(ctx, st.Then)
	if err != nil || !ok {
		slog.ErrorContext(ctx, "connector callback: the calendar on the shared consent has no usable OAuth app",
			"err", err, "provider", st.Then)
		land(outcomeError)
		return
	}
	auth, err := app.authenticate(ctx, consent)
	if err != nil {
		logConnectFailure(ctx, st.Then, err)
		land(connectFailureOutcome(st.Then, err))
		return
	}
	if _, err := h.registry.Connect(ctx, st.Then, auth); err != nil {
		slog.ErrorContext(ctx, "connector callback: persisting connection", "err", err, "provider", st.Then)
		land(outcomeError)
		return
	}
	land(outcomeOK)
}
