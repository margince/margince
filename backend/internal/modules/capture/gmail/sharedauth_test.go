// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/oauthflow"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const sharedMailbox = "team@example.com"

func serviceAccountKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	file, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"client_email": "reader@example-project.iam.gserviceaccount.com",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(file)
}

func sharedAuth(t *testing.T, key, mailbox string) connector.Auth {
	t.Helper()
	auth, err := SealShared(SharedCredential{ServiceAccountKey: key, Mailbox: mailbox})
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

// googleTokenEndpoint answers with a scripted status and body and records
// each form it received.
func googleTokenEndpoint(t *testing.T, status int, body string, forms *[]url.Values) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		*forms = append(*forms, r.PostForm)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("writing the token answer: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type sharedClock struct{ at time.Time }

func (c *sharedClock) now() time.Time { return c.at }

func assertionClaims(t *testing.T, assertion string) map[string]any {
	t.Helper()
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		t.Fatalf("the assertion has %d segments, want 3", len(parts))
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		t.Fatal(err)
	}
	return claims
}

func TestTheSharedAssertionActsForTheMailboxWithTheReadOnlyScopeAndIsCached(t *testing.T) {
	var forms []url.Values
	srv := googleTokenEndpoint(t, http.StatusOK, `{"access_token":"mbx-tok","expires_in":3599}`, &forms)
	clock := &sharedClock{at: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}
	authz := NewSharedAuthorizer(SharedAuthConfig{TokenURL: srv.URL, HTTPClient: srv.Client(), Now: clock.now})
	auth := sharedAuth(t, serviceAccountKey(t), sharedMailbox)

	for range 2 {
		tok, err := authz.AccessToken(context.Background(), auth)
		if err != nil || tok != "mbx-tok" {
			t.Fatalf("AccessToken = %q, %v", tok, err)
		}
	}
	if len(forms) != 1 {
		t.Fatalf("the token endpoint was asked %d times in one lifetime, want 1", len(forms))
	}
	if forms[0].Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
		t.Errorf("grant_type = %q, want the jwt-bearer grant", forms[0].Get("grant_type"))
	}
	claims := assertionClaims(t, forms[0].Get("assertion"))
	if claims["sub"] != sharedMailbox || claims["scope"] != "https://www.googleapis.com/auth/gmail.readonly" {
		t.Errorf("claims = %v, want sub %s and the read-only scope", claims, sharedMailbox)
	}
	if claims["iat"] != float64(clock.at.Unix()) {
		t.Errorf("iat = %v, want the injected clock's %d", claims["iat"], clock.at.Unix())
	}
	clock.at = clock.at.Add(time.Hour)
	if _, err := authz.AccessToken(context.Background(), auth); err != nil || len(forms) != 2 {
		t.Errorf("after expiry the token endpoint was asked %d times (%v), want a second mint", len(forms), err)
	}
}

func TestARefusedDelegationIsARefusedGrantNotARetry(t *testing.T) {
	var forms []url.Values
	srv := googleTokenEndpoint(t, http.StatusUnauthorized, `{"error":"unauthorized_client"}`, &forms)
	authz := NewSharedAuthorizer(SharedAuthConfig{TokenURL: srv.URL, HTTPClient: srv.Client(), Now: (&sharedClock{}).now})

	_, err := authz.AccessToken(context.Background(), sharedAuth(t, serviceAccountKey(t), sharedMailbox))
	if !errors.Is(err, connector.ErrAuthRejected) || errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("err = %v, want the auth-rejected class the registry parks on", err)
	}
	if !oauthflow.Misconfigured(err) {
		t.Errorf("Misconfigured(%v) = false, want the delegation named as the fault", err)
	}
}

func TestAnUnusableSharedBundleNeverReachesGoogle(t *testing.T) {
	var forms []url.Values
	srv := googleTokenEndpoint(t, http.StatusOK, `{"access_token":"x","expires_in":3599}`, &forms)
	authz := NewSharedAuthorizer(SharedAuthConfig{TokenURL: srv.URL, HTTPClient: srv.Client()})
	for name, auth := range map[string]connector.Auth{
		"no mailbox":   sharedAuth(t, serviceAccountKey(t), ""),
		"a path in it": sharedAuth(t, serviceAccountKey(t), "team/../boss@example.com"),
		"a named one":  sharedAuth(t, serviceAccountKey(t), "Team <team@example.com>"),
		"a broken key": sharedAuth(t, `{"type":"service_account"}`, sharedMailbox),
	} {
		if _, err := authz.AccessToken(context.Background(), auth); !errors.Is(err, connector.ErrAuthRejected) {
			t.Errorf("%s: err = %v, want a refusal", name, err)
		}
	}
	if len(forms) != 0 {
		t.Errorf("an unusable bundle reached the token endpoint %d times", len(forms))
	}
}
