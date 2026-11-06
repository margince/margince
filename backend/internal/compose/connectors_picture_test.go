// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const accountPicture = "https://lh3.googleusercontent.com/a/rep"

// fakeUserinfo is Google's userinfo endpoint: it counts the reads and answers status.
type fakeUserinfo struct {
	server *httptest.Server
	reads  atomic.Int32
	bearer atomic.Value
}

func newFakeUserinfo(t *testing.T, status int) *fakeUserinfo {
	t.Helper()
	f := &fakeUserinfo{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.reads.Add(1)
		f.bearer.Store(r.Header.Get("Authorization"))
		if r.URL.Path != "/userinfo" || status != http.StatusOK {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"sub":"1","picture":"` + accountPicture + `"}`)); err != nil {
			t.Errorf("answering userinfo: %v", err)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

type offeredPicture struct {
	user ids.UserID
	url  string
}

// pictureOfferBudget bounds the wait for a detached offer, so a missing one fails the test
// rather than hanging the package until its timeout.
const pictureOfferBudget = 30 * time.Second

func awaitOffer(t *testing.T, offered <-chan offeredPicture) offeredPicture {
	t.Helper()
	select {
	case got := <-offered:
		return got
	case <-time.After(pictureOfferBudget):
		t.Fatal("no picture was offered")
		return offeredPicture{}
	}
}

// mailboxOwnedBy is a Gmail API whose profile names owner as the connected account.
type mailboxOwnedBy struct {
	stubGmailAPI
	owner string
}

func (m mailboxOwnedBy) Profile(context.Context, string) (string, string, error) {
	return m.owner, "1", nil
}

// pictureConnect seals one Gmail grant of the account owner, under a seat signed in as seat.
func pictureConnect(t *testing.T, userinfo *fakeUserinfo, seat, owner string, granted []string) (connectorHandlers, *consentCode, connector.Auth, chan offeredPicture) {
	t.Helper()
	offered := make(chan offeredPicture, 1)
	h := wiredHandlers()
	h.oauth, h.gmailAPI = &recordingOAuth{granted: granted}, mailboxOwnedBy{owner: owner}
	h.picture = connectPicture{
		seatEmail: func(context.Context) (string, error) { return seat, nil },
		userinfo:  googleconn.NewUserinfo(userinfo.server.Client(), userinfo.server.URL),
		fill: func(_ context.Context, user ids.UserID, pictureURL string) error {
			offered <- offeredPicture{user, pictureURL}
			return nil
		},
	}
	consent := newConsentCode("the-code", h.callbackURL(providerGmail))
	return h, consent, authenticateWith(t, h, providerGmail, consent), offered
}

func TestAPersonalGoogleConnectOffersItsAccountPicture(t *testing.T) {
	userinfo := newFakeUserinfo(t, http.StatusOK)
	h, consent, auth, offered := pictureConnect(t, userinfo, "Rep@Example.com", "rep@example.com", gmailScopes)
	seat := ids.MustParse("22222222-2222-2222-2222-222222222222")

	h.offerConnectPicture(humanCtx(), seat, consent, auth)

	got := awaitOffer(t, offered)
	if got.user.UUID != seat || got.url != accountPicture {
		t.Errorf("offered %v at %q, want the granting seat and its account's picture", got.user, got.url)
	}
	if bearer := userinfo.bearer.Load(); bearer != "Bearer access" {
		t.Errorf("userinfo was read with %v, want the grant's own access token", bearer)
	}
}

func TestAGrantWithoutTheProfileScopeNeverReadsUserinfo(t *testing.T) {
	userinfo := newFakeUserinfo(t, http.StatusOK)
	withoutProfile := slices.DeleteFunc(slices.Clone(gmailScopes), func(s string) bool { return s == googleconn.ProfileScope })
	h, consent, auth, _ := pictureConnect(t, userinfo, "rep@example.com", "rep@example.com", withoutProfile)

	if _, ok := h.connectPictureSource(humanCtx(), consent, auth); ok {
		t.Error("a grant without the profile scope offers a picture lookup")
	}
	if n := userinfo.reads.Load(); n != 0 {
		t.Errorf("userinfo was read %d time(s), want none", n)
	}
}

func TestASharedMailboxConnectOffersNoPicture(t *testing.T) {
	userinfo := newFakeUserinfo(t, http.StatusOK)
	h, consent, auth, _ := pictureConnect(t, userinfo, "rep@example.com", "sales@example.com", gmailScopes)

	source, ok := h.connectPictureSource(humanCtx(), consent, auth)
	if !ok {
		t.Fatal("a grant with the profile scope offers no lookup at all")
	}
	pictureURL, err := source(humanCtx())

	if err != nil || pictureURL != "" {
		t.Errorf("a shared mailbox's lookup = %q, %v; want no picture and no error", pictureURL, err)
	}
	if n := userinfo.reads.Load(); n != 0 {
		t.Errorf("userinfo was read %d time(s) for an account that is not the seat's, want none", n)
	}
}

func TestAFailingUserinfoReportsRatherThanNamingAPicture(t *testing.T) {
	userinfo := newFakeUserinfo(t, http.StatusServiceUnavailable)
	h, consent, auth, _ := pictureConnect(t, userinfo, "rep@example.com", "rep@example.com", gmailScopes)

	source, ok := h.connectPictureSource(humanCtx(), consent, auth)
	if !ok {
		t.Fatal("a personal grant with the profile scope offers no lookup")
	}
	pictureURL, err := source(humanCtx())

	if !errors.Is(err, googleconn.ErrUnreachable) || pictureURL != "" {
		t.Errorf("a failing userinfo = %q, %v; want no picture and the unreachable error", pictureURL, err)
	}
}

func TestEveryGoogleConsentAsksForTheProfileScope(t *testing.T) {
	h := wiredHandlers()
	for _, provider := range []string{providerGmail, providerGcal} {
		_, u := consentState(t, h, authorizeURLOf(t, connectWith(t, h, provider, `{}`)))
		if scopes := strings.Fields(u.Query().Get("scope")); !slices.Contains(scopes, googleconn.ProfileScope) {
			t.Errorf("the %s consent asks for %v, without the profile scope", provider, scopes)
		}
	}
}

// Google refuses a refresh that asks for a scope the grant never held. An older grant
// must refresh without naming the configured list.
func TestAGrantFromBeforeTheProfileScopeStillRefreshes(t *testing.T) {
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := `{"access_token":"fresh","expires_in":3600}`
		if err := r.ParseForm(); err != nil || strings.Contains(r.PostForm.Get("scope"), googleconn.ProfileScope) {
			w.WriteHeader(http.StatusBadRequest)
			body = `{"error":"invalid_scope"}`
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("answering the refresh: %v", err)
		}
	}))
	defer token.Close()
	clients := map[string]googleconn.Authorizer{
		providerGmail: gmail.NewOAuth(gmail.OAuthConfig{ClientID: "cid", ClientSecret: "sec", Scopes: gmailScopes, TokenURL: token.URL}),
		providerGcal:  gcal.NewOAuth(gcal.OAuthConfig{ClientID: "cid", ClientSecret: "sec", TokenURL: token.URL}),
	}
	for provider, client := range clients {
		access, err := client.AccessToken(context.Background(), "granted-before-the-profile-scope")
		if err != nil || access != "fresh" {
			t.Errorf("%s refresh of an older grant = %q, %v; want a fresh access token", provider, access, err)
		}
	}
}

func TestOnlyTheSeatsOwnAddressIsItsMailbox(t *testing.T) {
	for _, tc := range []struct {
		seat, account string
		want          bool
	}{
		{"rep@example.com", "rep@example.com", true},
		{"Rep@Example.com ", "rep@example.com", true},
		{"rep@example.com", "sales@example.com", false},
		{"", "", false},
	} {
		if got := sameMailbox(tc.seat, tc.account); got != tc.want {
			t.Errorf("sameMailbox(%q, %q) = %v, want %v", tc.seat, tc.account, got, tc.want)
		}
	}
}
