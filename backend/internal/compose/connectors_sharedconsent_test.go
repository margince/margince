// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func authorizeURLOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp crmcontracts.ConnectConnectorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.AuthorizeUrl == nil {
		t.Fatalf("status %d, body %s: %v", rec.Code, rec.Body, err)
	}
	return *resp.AuthorizeUrl
}

func TestAGmailConnectWithItsCalendarAsksForBothOnOneScreen(t *testing.T) {
	h := wiredHandlers()
	st, u := consentState(t, h, authorizeURLOf(t, connectWith(t, h, "gmail", `{"also_connect":"gcal"}`)))

	scopes := strings.Fields(u.Query().Get("scope"))
	for _, want := range append(slices.Clone(gmailScopes), gcal.ConsentScopes()...) {
		if !slices.Contains(scopes, want) {
			t.Errorf("consent scope %v is missing %q", scopes, want)
		}
	}
	if got := u.Query().Get("redirect_uri"); got != "https://api.test/v1/connectors/gmail/callback" {
		t.Errorf("redirect_uri = %q, want the gmail callback, which connects both", got)
	}
	if !st.SharedConsent || st.Then != "gcal" {
		t.Errorf("state = %+v, want a shared consent connecting gcal", st)
	}
}

func TestAGmailConnectAloneAsksForMailOnly(t *testing.T) {
	h := wiredHandlers()
	st, u := consentState(t, h, authorizeURLOf(t, connectWith(t, h, "gmail", `{}`)))

	if scope := u.Query().Get("scope"); strings.Contains(scope, gcal.ReadScope) {
		t.Errorf("a mail-only consent asks for the calendar: %q", scope)
	}
	if st.SharedConsent {
		t.Error("a consent with nothing chained is marked shared")
	}
}

// Microsoft keeps its chained second consent; only a calendar listed in
// sharedConsentCalendars joins the mail screen.
func TestOnlyACalendarThatJoinsTheMailScreenSharesIt(t *testing.T) {
	for _, tc := range []struct {
		mail, calendar string
		want           bool
	}{
		{providerGmail, providerGcal, true},
		{providerGraph, providerGraphCal, false},
		{providerGcal, providerGcal, false},
		{providerGmail, "", false},
	} {
		if got := sharesConsent(tc.mail, tc.calendar); got != tc.want {
			t.Errorf("sharesConsent(%q, %q) = %v, want %v", tc.mail, tc.calendar, got, tc.want)
		}
	}
}

// Google's code is single use: the calendar seals the grant the mail leg
// redeemed, from the same refresh token, without a second exchange.
func TestBothGoogleConnectorsSealOneExchangedGrant(t *testing.T) {
	oauth := &recordingOAuth{granted: append(slices.Clone(gmailScopes), gcal.ConsentScopes()...)}
	h := wiredHandlers()
	h.oauth, h.gcalOAuth = oauth, oauth
	h.gmailAPI, h.gcalAPI = stubGmailAPI{}, stubGcalAPI{}
	consent := newConsentCode("the-code", h.callbackURL(providerGmail))

	mail := authenticateWith(t, h, providerGmail, consent)
	calendar := authenticateWith(t, h, providerGcal, consent)

	if oauth.exchanges != 1 {
		t.Fatalf("exchanged the code %d times, want once", oauth.exchanges)
	}
	if !consent.granted(gcal.ReadScope) {
		t.Error("the redeemed grant does not report the calendar scope it carries")
	}
	for name, auth := range map[string]connector.Auth{"gmail": mail, "gcal": calendar} {
		var sealed struct {
			RefreshToken string   `json:"refresh_token"`
			Granted      []string `json:"granted_scopes"`
		}
		if err := json.Unmarshal(auth, &sealed); err != nil {
			t.Fatalf("%s bundle: %v", name, err)
		}
		if sealed.RefreshToken != "refresh" || !slices.Contains(sealed.Granted, gcal.ReadScope) {
			t.Errorf("%s sealed %+v, want the one grant with what Google granted", name, sealed)
		}
	}
}

func authenticateWith(t *testing.T, h connectorHandlers, provider string, consent *consentCode) connector.Auth {
	t.Helper()
	app, ok, err := h.oauthApp(context.Background(), provider)
	if err != nil || !ok {
		t.Fatalf("%s app: ok=%v err=%v", provider, ok, err)
	}
	auth, err := app.authenticate(context.Background(), consent)
	if err != nil {
		t.Fatalf("%s authenticate: %v", provider, err)
	}
	return auth
}

// Google lets the user untick the calendar on the shared screen. The mailbox
// is already stored, so the landing names the declined calendar and keeps the
// held import, and nothing opens a second consent.
func TestAnUntickedCalendarLandsDeclinedWithoutASecondConsent(t *testing.T) {
	h := wiredHandlers()
	h.importer = (&recordedImport{estimate: connector.BackfillEstimate{Messages: autoImportCap * 3}}).engine()
	consent := newConsentCode("the-code", h.callbackURL(providerGmail))
	if _, err := consent.googleAuthorizer(&recordingOAuth{granted: gmailScopes}).Exchange(context.Background(), "the-code", ""); err != nil {
		t.Fatalf("exchange: %v", err)
	}
	rec := httptest.NewRecorder()

	h.chainAfterConnect(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), connectState{
		Workspace: chainWorkspace, User: chainUser, Provider: providerGmail, ReturnTo: returnToSettings,
		Then: providerGcal, SharedConsent: true, WindowMonths: 6,
	}, consent)

	if got, want := rec.Header().Get("Location"), "https://app.test/#/settings/connections/denied/gcal?import=6m"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

type stubGcalAPI struct{}

func (stubGcalAPI) PrimaryOwner(context.Context, string) (string, error) {
	return "owner@example.com", nil
}

func (stubGcalAPI) ListInitial(context.Context, string) ([][]byte, string, error) {
	return nil, "", nil
}

func (stubGcalAPI) ListIncremental(context.Context, string, string) ([][]byte, string, error) {
	return nil, "", nil
}
