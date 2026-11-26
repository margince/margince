// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const sharedUserID = "4a1d3c2b-0e9f-4a8b-8c7d-6e5f4a3b2c1d"

// sharedTenant answers the token endpoint and the two probe reads. Each
// status is what that call answers; zero is a success.
type sharedTenant struct {
	tokenStatus, userStatus, inboxStatus int

	mu    sync.Mutex
	calls []string
}

func (f *sharedTenant) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.calls = append(f.calls, r.URL.Path+"?"+r.URL.RawQuery)
		f.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			answer(w, f.tokenStatus, map[string]any{"access_token": "app-tok", "expires_in": 3599})
		case r.Header.Get("Authorization") != "Bearer app-tok":
			w.WriteHeader(http.StatusUnauthorized)
		case strings.HasSuffix(r.URL.Path, "/mailFolders/inbox"):
			answer(w, f.inboxStatus, map[string]any{"id": "inbox-id"})
		default:
			answer(w, f.userStatus, map[string]any{"id": sharedUserID})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func answer(w http.ResponseWriter, status int, body map[string]any) {
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, body)
}

func sharedConnector(srv *httptest.Server) *Connector {
	return New(nil, nil).WithSharedMailboxes(SharedMailboxConfig{
		Auth:       SharedAuthConfig{TokenURLFormat: srv.URL + "/%s/token", HTTPClient: srv.Client()},
		HTTPClient: srv.Client(), APIBase: srv.URL,
	})
}

func TestASharedProbeReadsTheUserIDAndOpensTheInbox(t *testing.T) {
	tenant := &sharedTenant{}
	srv := tenant.serve(t)
	probed, err := sharedConnector(srv).ProbeShared(context.Background(), sharedBundle(t, "contoso.example"))
	if err != nil {
		t.Fatal(err)
	}
	cred, err := ReadShared(probed)
	if err != nil || cred.UserID != sharedUserID || cred.ClientSecret != "s3cret" {
		t.Fatalf("probed bundle = %+v, %v; want the user id beside the sealed app", cred, err)
	}
	want := []string{
		"/contoso.example/token?",
		"/users/" + sharedAddress + "?%24select=id",
		"/users/" + sharedAddress + "/mailFolders/inbox?%24select=id",
	}
	if strings.Join(tenant.calls, " ") != strings.Join(want, " ") {
		t.Errorf("probe calls = %v, want %v", tenant.calls, want)
	}
}

func TestARefusedSharedProbeNamesItsClassAndNeverTheSecret(t *testing.T) {
	for name, tc := range map[string]struct {
		tenant     string
		fake       *sharedTenant
		wantReason string
	}{
		"multi-tenant alias": {"common", &sharedTenant{}, connector.SharedCredentialsInvalid},
		"refused app":        {"contoso.example", &sharedTenant{tokenStatus: http.StatusUnauthorized}, connector.SharedGrantRefused},
		"no mailbox":         {"contoso.example", &sharedTenant{userStatus: http.StatusNotFound}, connector.SharedMailboxMissing},
		"outside its scope":  {"contoso.example", &sharedTenant{inboxStatus: http.StatusForbidden}, connector.SharedMailboxDenied},
		"no directory read":  {"contoso.example", &sharedTenant{userStatus: http.StatusForbidden}, connector.SharedMailboxDenied},
	} {
		t.Run(name, func(t *testing.T) {
			srv := tc.fake.serve(t)
			_, err := sharedConnector(srv).ProbeShared(context.Background(), sharedBundle(t, tc.tenant))
			var refusal *connector.SharedMailboxRefusal
			if !errors.As(err, &refusal) || refusal.Reason != tc.wantReason {
				t.Fatalf("err = %v, want a %s refusal", err, tc.wantReason)
			}
			if strings.Contains(err.Error(), "s3cret") {
				t.Errorf("the refusal carries the secret: %v", err)
			}
		})
	}
	srv := (&sharedTenant{inboxStatus: http.StatusServiceUnavailable}).serve(t)
	_, err := sharedConnector(srv).ProbeShared(context.Background(), sharedBundle(t, "contoso.example"))
	if !errors.Is(err, connector.ErrUnreachable) {
		t.Errorf("a down mailbox answered %v, want unreachable, never a refusal", err)
	}
}

func TestASharedLinkMayNameTheMailboxByAnyOfItsForms(t *testing.T) {
	g := &recordingGraph{}
	srv := g.serve(t)
	shared, err := NewSharedAPI(srv.Client(), srv.URL, sharedAddress, sharedUserID)
	if err != nil {
		t.Fatal(err)
	}
	after := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for _, inside := range []string{
		"/users/" + sharedUserID + "/messages?$skip=10",
		"/users/" + strings.ToUpper(sharedUserID) + "/messages?$skip=10",
		"/Users('" + sharedAddress + "')/messages?$skip=10",
		"/users('" + sharedUserID + "')/messages?$skip=10",
		"/users/team%40contoso.example/messages?$skip=10",
	} {
		if _, _, err := shared.ListAfter(context.Background(), "t", after, srv.URL+inside, 10); err != nil {
			t.Errorf("%s was refused: %v", inside, err)
		}
	}
	for _, outside := range []string{
		"/users/5b2e4d3c-1fa0-4b9c-9d8e-7f6a5b4c3d2e/messages",
		"/users('other@contoso.example')/messages",
		"/users('" + sharedUserID + "')x/messages",
		"/users/" + sharedUserID + "/../other@contoso.example/messages",
	} {
		if _, _, err := shared.ListAfter(context.Background(), "t", after, srv.URL+outside, 10); !errors.Is(err, ErrUnreachable) {
			t.Errorf("%s = %v, want refused", outside, err)
		}
	}
	if _, err := NewSharedAPI(nil, "", sharedAddress, "../other"); err == nil {
		t.Error("a user id that is not an object id was accepted")
	}
}

func TestASharedSubscriptionMatchesEveryFormOfItsResource(t *testing.T) {
	api, err := newSharedHTTPAPI(nil, "", sharedAddress, sharedUserID)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range []string{
		"/users/" + sharedAddress + "/messages", "users/" + sharedUserID + "/messages",
		"Users('" + strings.ToUpper(sharedUserID) + "')/Messages", "Users('" + sharedAddress + "')/messages",
		"users/team%40contoso.example/messages",
	} {
		if !api.watchesThisMailbox(resource) {
			t.Errorf("%s was not matched", resource)
		}
	}
	for _, resource := range []string{
		"users/5b2e4d3c-1fa0-4b9c-9d8e-7f6a5b4c3d2e/messages", "Users('other@contoso.example')/messages",
		"users/" + sharedUserID + "/events", "users/" + sharedUserID + "/messages/x",
	} {
		if api.watchesThisMailbox(resource) {
			t.Errorf("%s was matched", resource)
		}
	}
}

func TestAnOpenedSharedConnectorIsAskedAsTheMailboxAndNeverRotates(t *testing.T) {
	srv := (&sharedTenant{}).serve(t)
	bundle, err := SealShared(SharedCredential{
		TenantID: "contoso.example", ClientID: "app-1", ClientSecret: "s3cret", Mailbox: sharedAddress, UserID: sharedUserID,
	})
	if err != nil {
		t.Fatal(err)
	}
	opened, state, err := sharedConnector(srv).OpenShared(bundle)
	if err != nil {
		t.Fatal(err)
	}
	label, err := opened.(*Connector).AccountLabel(state)
	if err != nil || label != sharedAddress {
		t.Errorf("AccountLabel = %q, %v; want the shared address", label, err)
	}
	if strings.Contains(string(state), "s3cret") {
		t.Error("the run's auth state carries the app secret")
	}
	refreshed, err := opened.(*Connector).oauth.Refresh(context.Background(), "", nil)
	if err != nil || refreshed.AccessToken != "app-tok" || refreshed.Rotated != "" {
		t.Errorf("Refresh = %+v, %v; want the app token and no rotation", refreshed, err)
	}
	if _, _, err := New(nil, nil).OpenShared(bundle); !errors.Is(err, errNoSharedReader) {
		t.Errorf("a connector with no shared reader opened a bundle: %v", err)
	}
}

// subscriptionsByID serves one stored subscription by id, and records each
// renewal and creation it is asked for.
type subscriptionsByID struct {
	stored   map[string]any
	renewals []string
	created  int
}

func (f *subscriptionsByID) serve(t *testing.T) *httptest.Server {
	t.Helper()
	expiry := clocktest.Now(t).Add(time.Hour).UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/token"):
			writeJSON(w, map[string]any{"access_token": "app-tok", "expires_in": 3599})
		case r.URL.Path == "/subscriptions" && r.Method == http.MethodPost:
			f.created++
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]any{"id": "sub-new", "expirationDateTime": expiry})
		case r.URL.Path == "/subscriptions":
			writeJSON(w, map[string]any{"value": []any{}})
		case r.Method == http.MethodPatch:
			f.renewals = append(f.renewals, r.URL.Path)
			writeJSON(w, map[string]any{"id": f.stored["id"], "expirationDateTime": expiry})
		default:
			writeJSON(w, f.stored)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAStoredSubscriptionOfAnotherMailboxIsNotRenewedForASharedOne(t *testing.T) {
	const hook = "https://api.example/webhooks/graph?token=t"
	for name, tc := range map[string]struct {
		resource  string
		wantRenew bool
	}{
		"another mailbox's":        {"/users/other@contoso.example/messages", false},
		"its own, by object id":    {"Users('" + sharedUserID + "')/messages", true},
		"its own, by address form": {"users/" + sharedAddress + "/messages", true},
	} {
		t.Run(name, func(t *testing.T) {
			fake := &subscriptionsByID{stored: map[string]any{"id": "sub-1", "resource": tc.resource, "notificationUrl": hook}}
			srv := fake.serve(t)
			bundle, err := SealShared(SharedCredential{
				TenantID: "contoso.example", ClientID: "app-1", ClientSecret: "s3cret", Mailbox: sharedAddress, UserID: sharedUserID,
			})
			if err != nil {
				t.Fatal(err)
			}
			opened, state, err := sharedConnector(srv).OpenShared(bundle)
			if err != nil {
				t.Fatal(err)
			}
			res, err := opened.(*Connector).RenewWatch(context.Background(), state, hook, "sub-1")
			if err != nil {
				t.Fatal(err)
			}
			renewed := len(fake.renewals) == 1 && fake.created == 0 && res.Ref == "sub-1"
			created := len(fake.renewals) == 0 && fake.created == 1 && res.Ref == "sub-new"
			if (tc.wantRenew && !renewed) || (!tc.wantRenew && !created) {
				t.Errorf("renewals %v, created %d, ref %q; want renew = %v", fake.renewals, fake.created, res.Ref, tc.wantRenew)
			}
		})
	}
	// A personal token reads only its own user's subscriptions, so the URL
	// alone still decides.
	fake := &subscriptionsByID{stored: map[string]any{"id": "sub-1", "resource": "Users('x')/messages", "notificationUrl": hook}}
	srv := fake.serve(t)
	if _, err := NewAPI(srv.Client(), srv.URL).GetSubscription(context.Background(), "t", "sub-1"); err != nil {
		t.Errorf("a personal client refused its stored subscription: %v", err)
	}
}
