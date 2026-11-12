// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gdocs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/googleconn"
	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type fakeOAuth struct {
	granted   []string
	accessErr error
}

func (f fakeOAuth) Exchange(context.Context, string, string) (oauthflow.TokenGrant, error) {
	return oauthflow.TokenGrant{RefreshToken: "refresh-1", Scopes: f.granted}, nil
}

func (f fakeOAuth) AccessToken(context.Context, string) (string, error) {
	return "access-1", f.accessErr
}

// userinfoServer is Google's userinfo endpoint, answering one address for the
// one access token the fake OAuth mints.
func userinfoServer(t *testing.T, email string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/userinfo" || r.Header.Get("Authorization") != "Bearer access-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]string{"email": email}); err != nil {
			t.Errorf("writing userinfo: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func authenticate(t *testing.T, granted []string) connector.Auth {
	t.Helper()
	srv := userinfoServer(t, "ada@example.com")
	c := New(fakeOAuth{granted: granted}, NewOwner(srv.Client(), srv.URL))
	req, err := AuthRequestFrom("code-1", "https://crm.example/v1/connectors/gdocs/callback")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := c.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	return auth
}

func TestAuthenticateNamesTheAccountAndKeepsTheGrant(t *testing.T) {
	t.Parallel()
	auth := authenticate(t, ConsentScopes())
	c := New(fakeOAuth{}, nil)
	label, err := c.AccountLabel(auth)
	if err != nil || label != "ada@example.com" {
		t.Fatalf("AccountLabel = %q, %v; want ada@example.com", label, err)
	}
	granted, err := c.GrantedScopes(auth)
	if err != nil || !slices.Equal(granted, ConsentScopes()) {
		t.Fatalf("GrantedScopes = %v, %v; want %v", granted, err, ConsentScopes())
	}
}

func TestAuthenticateRefusesAnAccountWithNoAddress(t *testing.T) {
	t.Parallel()
	srv := userinfoServer(t, "")
	c := New(fakeOAuth{granted: ConsentScopes()}, NewOwner(srv.Client(), srv.URL))
	req, err := AuthRequestFrom("code-1", "https://crm.example/cb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Authenticate(context.Background(), req); !errors.Is(err, googleconn.ErrAuthRejected) {
		t.Fatalf("Authenticate with no address = %v; want ErrAuthRejected", err)
	}
}

func TestSyncCapturesNothingAndKeepsTheCursor(t *testing.T) {
	t.Parallel()
	auth := authenticate(t, ConsentScopes())
	cursor := connector.Cursor(`{"kept":true}`)
	got, err := New(fakeOAuth{}, nil).Sync(context.Background(), auth, cursor, nil)
	if err != nil || string(got) != string(cursor) {
		t.Fatalf("Sync = %s, %v; want the cursor back unchanged", got, err)
	}
}

func TestSyncReportsARevokedGrant(t *testing.T) {
	t.Parallel()
	auth := authenticate(t, ConsentScopes())
	revoked := fakeOAuth{accessErr: googleconn.ErrAuthRejected}
	if _, err := New(revoked, nil).Sync(context.Background(), auth, nil, nil); !errors.Is(err, googleconn.ErrAuthRejected) {
		t.Fatalf("Sync with a revoked grant = %v; want ErrAuthRejected", err)
	}
	if err := New(revoked, nil).HealthCheck(context.Background(), auth); !errors.Is(err, googleconn.ErrAuthRejected) {
		t.Fatalf("HealthCheck with a revoked grant = %v; want ErrAuthRejected", err)
	}
}

func TestTheConsentAsksForDocsReadAndTheAddressOnly(t *testing.T) {
	t.Parallel()
	consent := NewOAuth(OAuthConfig{ClientID: "client-1"}).AuthCodeURL("state-1", "https://crm.example/cb")
	parsed, err := url.Parse(consent)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	scopes := strings.Fields(q.Get("scope"))
	if !slices.Equal(scopes, []string{ReadScope, EmailScope}) {
		t.Errorf("consent scopes = %v; want documents.readonly and userinfo.email", scopes)
	}
	if q.Has("include_granted_scopes") {
		t.Error("the consent gathers earlier Google grants into the Docs credential")
	}
	if q.Get("access_type") != "offline" || q.Get("prompt") != "consent" {
		t.Errorf("consent lacks offline access: %v", q)
	}
}
