// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const sharedAddress = "team@contoso.example"

// recordingGraph answers every call with an empty success and records each
// request's path, and the resource a subscription is created for.
type recordingGraph struct {
	mu        sync.Mutex
	paths     []string
	resources []string
}

func (g *recordingGraph) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the request body: %v", err)
		}
		var sub struct {
			Resource string `json:"resource"`
		}
		g.mu.Lock()
		g.paths = append(g.paths, r.URL.EscapedPath())
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, subscriptionsPath) && json.Unmarshal(body, &sub) == nil {
			g.resources = append(g.resources, sub.Resource)
		}
		g.mu.Unlock()
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/sendMail") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeJSON(w, map[string]any{
			"value": []any{}, "id": "x", "mail": sharedAddress, "parentFolderId": "f1", "@odata.count": 0,
			"@odata.deltaLink": "http://" + r.Host + r.URL.Path + "?$deltatoken=d1",
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (g *recordingGraph) requests() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.paths...)
}

// Each mailbox call of the API, so a call added later without a shared
// spelling shows up here as a request to the wrong root.
func everyMailboxCall(api API) map[string]func(context.Context) error {
	noVisit := func(context.Context, DeltaPage) error { return nil }
	after := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	return map[string]func(context.Context) error{
		"Profile": func(ctx context.Context) error { _, err := api.Profile(ctx, "t"); return err },
		"DeltaInit": func(ctx context.Context) error {
			_, err := api.DeltaInit(ctx, "t", folderInbox, after, nil, noVisit)
			return err
		},
		"DeletionsDelta": func(ctx context.Context) error {
			_, err := api.DeletionsDelta(ctx, "t", "", noVisit)
			return err
		},
		"ListDeletions": func(ctx context.Context) error { _, err := api.ListDeletions(ctx, "t"); return err },
		"MessageFolder": func(ctx context.Context) error { _, _, err := api.MessageFolder(ctx, "t", "m1"); return err },
		"FindMessage":   func(ctx context.Context) error { _, err := api.FindMessage(ctx, "t", "<a@b>"); return err },
		"EnsureSubscription": func(ctx context.Context) error {
			_, err := api.EnsureSubscription(ctx, "t", "https://crm.example/hook", "state", after.Add(time.Hour))
			return err
		},
		"GetMIME":       func(ctx context.Context) error { _, err := api.GetMIME(ctx, "t", "f1", "m1"); return err },
		"EstimateAfter": func(ctx context.Context) error { _, err := api.EstimateAfter(ctx, "t", after); return err },
		"ListAfter":     func(ctx context.Context) error { _, _, err := api.ListAfter(ctx, "t", after, "", 10); return err },
		"FolderID":      func(ctx context.Context) error { _, err := api.FolderID(ctx, "t", "sentitems"); return err },
		"ListFolders":   func(ctx context.Context) error { _, _, err := api.ListFolders(ctx, "t"); return err },
		"SendMIME":      func(ctx context.Context) error { return api.SendMIME(ctx, "t", []byte("Subject: hi\r\n\r\nbody")) },
		"FindSentByMessageID": func(ctx context.Context) error {
			_, _, err := api.FindSentByMessageID(ctx, "t", "a@b")
			return err
		},
	}
}

func TestEveryCallOfASharedMailboxGoesToItsUserRootAndNoneToMe(t *testing.T) {
	g := &recordingGraph{}
	srv := g.serve(t)
	api, err := NewSharedAPI(srv.Client(), srv.URL, sharedAddress)
	if err != nil {
		t.Fatal(err)
	}
	root := "/users/" + url.PathEscape(sharedAddress)
	for name, call := range everyMailboxCall(api) {
		before := len(g.requests())
		// The empty answers fail some calls; only where they were sent matters.
		if err := call(context.Background()); err != nil {
			t.Logf("%s: %v", name, err)
		}
		sent := g.requests()[before:]
		if len(sent) == 0 {
			t.Errorf("%s sent no request, so this test proves nothing about it", name)
		}
		for _, path := range sent {
			if path != subscriptionsPath && path != root && !strings.HasPrefix(path, root+"/") {
				t.Errorf("%s requested %s, want a path under %s", name, path, root)
			}
		}
	}
	if len(g.resources) != 1 || g.resources[0] != root+"/messages" {
		t.Errorf("subscription resources = %v, want [%s/messages]", g.resources, root)
	}
}

func TestAPersonalMailboxStillCallsMe(t *testing.T) {
	g := &recordingGraph{}
	srv := g.serve(t)
	if err := NewAPI(srv.Client(), srv.URL).SendMIME(context.Background(), "t", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAPI(srv.Client(), srv.URL).Profile(context.Background(), "t"); err != nil {
		t.Fatal(err)
	}
	if got := g.requests(); len(got) != 2 || got[0] != "/me/sendMail" || got[1] != "/me" {
		t.Errorf("personal requests = %v, want [/me/sendMail /me]", got)
	}
}

func TestASharedMailboxAddressThatCouldShapeThePathIsRefused(t *testing.T) {
	for _, address := range []string{"", "team", "Team <team@contoso.example>", "team/../me@contoso.example", "team%2f@contoso.example"} {
		if _, err := NewSharedAPI(nil, "", address); !errors.Is(err, oauthflow.ErrMailboxAddress) {
			t.Errorf("NewSharedAPI(%q) = %v, want ErrMailboxAddress", address, err)
		}
	}
}

func TestASharedContinuationLinkMustStayInsideItsMailbox(t *testing.T) {
	g := &recordingGraph{}
	srv := g.serve(t)
	shared, err := NewSharedAPI(srv.Client(), srv.URL, sharedAddress)
	if err != nil {
		t.Fatal(err)
	}
	after := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for _, link := range []string{
		srv.URL + "/me/messages?$skip=10",
		srv.URL + "/users/other@contoso.example/messages?$skip=10",
		srv.URL + "/users/" + sharedAddress + "/../other@contoso.example/messages",
		srv.URL + "/users/" + sharedAddress + "x/messages",
	} {
		if _, _, err := shared.ListAfter(context.Background(), "t", after, link, 10); !errors.Is(err, ErrUnreachable) {
			t.Errorf("shared ListAfter(%s) = %v, want a provider error", link, err)
		}
	}
	if sent := g.requests(); len(sent) != 0 {
		t.Errorf("a refused link was followed: %v", sent)
	}
	inside := srv.URL + "/Users/" + sharedAddress + "/messages?$skip=10"
	if _, _, err := shared.ListAfter(context.Background(), "t", after, inside, 10); err != nil {
		t.Errorf("a link inside the mailbox was refused: %v", err)
	}
	// A personal client keeps the origin-only check.
	if _, _, err := NewAPI(srv.Client(), srv.URL).ListAfter(context.Background(), "t", after, srv.URL+"/users/x/messages", 10); err != nil {
		t.Errorf("a personal client refused an on-origin link: %v", err)
	}
}

func TestASharedMailboxAdoptsOnlyItsOwnSubscription(t *testing.T) {
	const hook = "https://api.example/webhooks/graph?token=t"
	other := subscription{ID: "sub-other", Resource: "/users/other@contoso.example/messages", NotificationURL: hook}
	ours := subscription{ID: "sub-ours", Resource: "Users/" + strings.ToUpper(sharedAddress) + "/Messages", NotificationURL: hook}
	for name, tc := range map[string]struct {
		existing  []subscription
		wantRenew bool
	}{
		"another mailbox's only": {[]subscription{other}, false},
		"its own beside another": {[]subscription{other, ours}, true},
	} {
		t.Run(name, func(t *testing.T) {
			var created, renewed int
			srv := subscriptionStub(t, subscriptionStubState{
				existing: tc.existing, onCreate: func() { created++ }, onRenew: func() { renewed++ },
			})
			api, err := NewSharedAPI(srv.Client(), srv.URL, sharedAddress)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := api.EnsureSubscription(context.Background(), "at", hook, sharedAddress, clocktest.Now(t).Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			if got := renewed == 1 && created == 0; got != tc.wantRenew || renewed+created != 1 {
				t.Errorf("renewed %d, created %d; want renew = %v", renewed, created, tc.wantRenew)
			}
		})
	}
	// A personal client still matches on the URL alone.
	var renewed int
	srv := subscriptionStub(t, subscriptionStubState{existing: []subscription{other}, onRenew: func() { renewed++ }})
	if _, err := NewAPI(srv.Client(), srv.URL).EnsureSubscription(context.Background(), "at", hook, owner, clocktest.Now(t).Add(time.Hour)); err != nil || renewed != 1 {
		t.Errorf("personal renewed %d (%v), want the URL match renewed", renewed, err)
	}
}

// tokenEndpoint is a fake Microsoft identity endpoint that answers with a
// scripted status and body, and records each request.
type tokenEndpoint struct {
	status int
	body   string
	paths  []string
	forms  []url.Values
}

func (e *tokenEndpoint) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		e.paths = append(e.paths, r.URL.Path)
		e.forms = append(e.forms, r.PostForm)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(e.status)
		if _, err := w.Write([]byte(e.body)); err != nil {
			t.Errorf("writing the token answer: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type stepClock struct{ at time.Time }

func (c *stepClock) now() time.Time { return c.at }

func sharedBundle(t *testing.T, tenant string) connector.Auth {
	t.Helper()
	auth, err := SealShared(SharedCredential{
		TenantID: tenant, ClientID: "app-1", ClientSecret: "s3cret", Mailbox: sharedAddress,
	})
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func TestTheSharedTokenRequestNamesTheTenantAndDefaultScopeAndIsCached(t *testing.T) {
	endpoint := &tokenEndpoint{status: http.StatusOK, body: `{"access_token":"app-tok","expires_in":3599}`}
	srv := endpoint.serve(t)
	clock := &stepClock{at: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}
	authz := NewSharedAuthorizer(SharedAuthConfig{
		TokenURLFormat: srv.URL + "/%s/oauth2/v2.0/token", HTTPClient: srv.Client(), Now: clock.now,
	})
	tenant := "8f0c2a7e-1b2c-4d5e-9f00-112233445566"

	for _, at := range []time.Duration{0, 30 * time.Minute} {
		clock.at = clock.at.Add(at)
		tok, err := authz.AccessToken(context.Background(), sharedBundle(t, tenant))
		if err != nil || tok != "app-tok" {
			t.Fatalf("AccessToken = %q, %v", tok, err)
		}
	}
	if len(endpoint.paths) != 1 {
		t.Fatalf("the token endpoint was asked %d times in one lifetime, want 1", len(endpoint.paths))
	}
	if want := "/" + tenant + "/oauth2/v2.0/token"; endpoint.paths[0] != want {
		t.Errorf("token path = %s, want %s", endpoint.paths[0], want)
	}
	form := endpoint.forms[0]
	if form.Get("grant_type") != "client_credentials" || form.Get("scope") != sharedScope || form.Get("client_id") != "app-1" {
		t.Errorf("token form = %v, want the client-credentials grant for .default", form)
	}
	clock.at = clock.at.Add(time.Hour)
	if _, err := authz.AccessToken(context.Background(), sharedBundle(t, tenant)); err != nil || len(endpoint.paths) != 2 {
		t.Errorf("after expiry the token endpoint was asked %d times (%v), want a second mint", len(endpoint.paths), err)
	}
}

func TestARefusedAppIsARefusedGrantNotARetry(t *testing.T) {
	for _, code := range []string{"invalid_client", "unauthorized_client"} {
		t.Run(code, func(t *testing.T) {
			endpoint := &tokenEndpoint{status: http.StatusUnauthorized, body: `{"error":"` + code + `"}`}
			srv := endpoint.serve(t)
			authz := NewSharedAuthorizer(SharedAuthConfig{
				TokenURLFormat: srv.URL + "/%s/token", HTTPClient: srv.Client(), Now: (&stepClock{}).now,
			})

			_, err := authz.AccessToken(context.Background(), sharedBundle(t, "contoso.example"))
			if !errors.Is(err, connector.ErrAuthRejected) || errors.Is(err, connector.ErrUnreachable) {
				t.Fatalf("err = %v, want the auth-rejected class the registry parks on", err)
			}
			if !oauthflow.Misconfigured(err) {
				t.Errorf("Misconfigured(%v) = false, want the app named as the fault", err)
			}
		})
	}
}

func TestASharedBundleNamingNoOwnTenantNeverReachesMicrosoft(t *testing.T) {
	endpoint := &tokenEndpoint{status: http.StatusOK, body: `{"access_token":"x","expires_in":3599}`}
	srv := endpoint.serve(t)
	authz := NewSharedAuthorizer(SharedAuthConfig{TokenURLFormat: srv.URL + "/%s/token", HTTPClient: srv.Client()})
	for _, tenant := range []string{
		"", "common", "Organizations", "consumers", "contoso", "contoso.example/../common", "a/b.example",
		"contoso..example", ".contoso.example", "contoso.example.", "contoso.example%2f", "con toso.example",
		"8f0c2a7e-1b2c-4d5e-9f00-11223344556", "..",
	} {
		if _, err := authz.AccessToken(context.Background(), sharedBundle(t, tenant)); !errors.Is(err, connector.ErrAuthRejected) {
			t.Errorf("tenant %q: err = %v, want a refusal", tenant, err)
		}
	}
	if len(endpoint.paths) != 0 {
		t.Errorf("a multi-tenant alias reached the token endpoint: %v", endpoint.paths)
	}
}
