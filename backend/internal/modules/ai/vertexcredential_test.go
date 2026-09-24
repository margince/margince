// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	testVertexKeyOnce sync.Once
	testVertexKey     *rsa.PrivateKey
)

func vertexTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	testVertexKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generating the test key: %v", err)
		}
		testVertexKey = key
	})
	return testVertexKey
}

func pkcs8PEM(t *testing.T, key crypto.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("encoding the test key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// serviceAccountJSON is a key file shaped like Google's, with overrides.
func serviceAccountJSON(t *testing.T, overrides map[string]string) string {
	t.Helper()
	file := map[string]string{
		"type":         "service_account",
		"project_id":   "margince-eu-1",
		"private_key":  pkcs8PEM(t, vertexTestKey(t)),
		"client_email": "crm@margince-eu-1.iam.gserviceaccount.com",
		"token_uri":    vertexTokenURI,
	}
	for field, value := range overrides {
		if value == "" {
			delete(file, field)
			continue
		}
		file[field] = value
	}
	raw, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("encoding the key file: %v", err)
	}
	return string(raw)
}

func TestAVertexAssertionIsSignedForThePinnedTokenEndpoint(t *testing.T) {
	t.Parallel()
	pkcs1 := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(vertexTestKey(t))}))
	for name, keyFile := range map[string]string{
		"a PKCS#8 key": serviceAccountJSON(t, nil),
		"a PKCS#1 key": serviceAccountJSON(t, map[string]string{"private_key": pkcs1}),
		"no token_uri": serviceAccountJSON(t, map[string]string{"token_uri": ""}),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			account, err := parseVertexServiceAccount(keyFile)
			if err != nil {
				t.Fatalf("a well-formed key file was refused: %v", err)
			}
			issued := time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
			jwt, err := account.assertion(issued)
			if err != nil {
				t.Fatal(err)
			}
			parts := strings.Split(jwt, ".")
			if len(parts) != 3 {
				t.Fatalf("the assertion has %d segments, want 3", len(parts))
			}
			signature, err := base64.RawURLEncoding.DecodeString(parts[2])
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
			if err := rsa.VerifyPKCS1v15(&vertexTestKey(t).PublicKey, crypto.SHA256, digest[:], signature); err != nil {
				t.Fatalf("the signature does not verify with the key's public half: %v", err)
			}
			type claimSet struct {
				Iss, Aud, Scope string
				Iat, Exp        int64
			}
			var claims claimSet
			decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
			if err != nil || json.Unmarshal(decoded, &claims) != nil {
				t.Fatalf("the claims segment is not JSON: %v", err)
			}
			want := claimSet{
				Iss:   "crm@margince-eu-1.iam.gserviceaccount.com",
				Aud:   "https://oauth2.googleapis.com/token",
				Scope: "https://www.googleapis.com/auth/cloud-platform",
				Iat:   issued.Unix(),
				Exp:   issued.Add(time.Hour).Unix(),
			}
			if claims != want {
				t.Errorf("claims = %+v, want %+v", claims, want)
			}
		})
	}
}

func TestAnUnusableServiceAccountKeyIsRefusedByField(t *testing.T) {
	t.Parallel()
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		keyFile, names string
	}{
		"not JSON":                {"-----BEGIN PRIVATE KEY-----", "not a JSON object"},
		"another credential type": {serviceAccountJSON(t, map[string]string{"type": "authorized_user"}), "type"},
		"no client_email":         {serviceAccountJSON(t, map[string]string{"client_email": ""}), "client_email"},
		"no project_id":           {serviceAccountJSON(t, map[string]string{"project_id": ""}), "project_id"},
		"a project_id with a path": {
			serviceAccountJSON(t, map[string]string{"project_id": "evil/../../v1/x"}), "project_id",
		},
		"an uppercase project_id":   {serviceAccountJSON(t, map[string]string{"project_id": "Margince-EU"}), "project_id"},
		"a trailing hyphen":         {serviceAccountJSON(t, map[string]string{"project_id": "margince-eu-"}), "project_id"},
		"another token endpoint":    {serviceAccountJSON(t, map[string]string{"token_uri": "https://attacker.example/token"}), "token_uri"},
		"a plain-http token uri":    {serviceAccountJSON(t, map[string]string{"token_uri": "http://oauth2.googleapis.com/token"}), "token_uri"},
		"no private_key":            {serviceAccountJSON(t, map[string]string{"private_key": ""}), "private_key"},
		"a private_key that is not": {serviceAccountJSON(t, map[string]string{"private_key": "not a pem"}), "private_key"},
		"an EC key":                 {serviceAccountJSON(t, map[string]string{"private_key": pkcs8PEM(t, ecKey)}), "private_key"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := parseVertexServiceAccount(tc.keyFile)
			if !errors.Is(err, errInvalidServiceAccount) || !strings.Contains(err.Error(), tc.names) {
				t.Fatalf("want a refusal naming %s, got %v", tc.names, err)
			}
			for _, leaked := range []string{"BEGIN", "attacker.example", "evil/", "Margince-EU"} {
				if strings.Contains(err.Error(), leaked) {
					t.Errorf("the refusal echoes the value it was given (%q): %v", leaked, err)
				}
			}
		})
	}
}

// tokenEndpoint stands in for Google's token host at the transport, which is
// the only way to reach a URL the code pins.
type tokenEndpoint struct {
	respond   func(*http.Request) tokenReply
	exchanges atomic.Int32
	elsewhere atomic.Int32
}

func (e *tokenEndpoint) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() != vertexTokenURI {
		e.elsewhere.Add(1)
		return tokenReply{status: http.StatusOK, body: `{}`}.response(), nil
	}
	e.exchanges.Add(1)
	return e.respond(req).response(), nil
}

type tokenReply struct {
	status         int
	body, location string
}

func (r tokenReply) response() *http.Response {
	header := http.Header{"Content-Type": {"application/json"}}
	if r.location != "" {
		header.Set("Location", r.location)
	}
	return &http.Response{StatusCode: r.status, Header: header, Body: io.NopCloser(strings.NewReader(r.body))}
}

func grantedToken(token string) func(*http.Request) tokenReply {
	return func(*http.Request) tokenReply {
		return tokenReply{status: http.StatusOK, body: `{"access_token":"` + token + `","expires_in":3600,"token_type":"Bearer"}`}
	}
}

func testTokenSource(t *testing.T, endpoint http.RoundTripper, clock Clock) *vertexTokenSource {
	t.Helper()
	account, err := parseVertexServiceAccount(serviceAccountJSON(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	return newVertexTokenSource(account, &http.Client{Transport: endpoint}, clock)
}

func TestTheExchangeSendsAJWTBearerGrant(t *testing.T) {
	t.Parallel()
	var form url.Values
	endpoint := &tokenEndpoint{respond: func(req *http.Request) tokenReply {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("reading the exchange body: %v", err)
		}
		form, err = url.ParseQuery(string(raw))
		if err != nil || req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("the exchange is not a form post: %v", err)
		}
		return grantedToken("ya29.minted")(req)
	}}
	source := testTokenSource(t, endpoint, &fixedClock{now: time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)})
	token, err := source.accessToken(context.Background())
	if err != nil || token != "ya29.minted" {
		t.Fatalf("token %q, err %v", token, err)
	}
	if form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || strings.Count(form.Get("assertion"), ".") != 2 {
		t.Errorf("the exchange form is %v, want a jwt-bearer grant carrying a JWT", form)
	}
}

func TestTheAccessTokenIsReusedUntilFiveMinutesBeforeItExpires(t *testing.T) {
	t.Parallel()
	clock := &fixedClock{now: time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)}
	endpoint := &tokenEndpoint{respond: grantedToken("ya29.t")}
	source := testTokenSource(t, endpoint, clock)
	for _, step := range []struct {
		advance time.Duration
		want    int32
	}{
		{0, 1},
		{54 * time.Minute, 1},
		{time.Minute - time.Second, 1},
		{time.Second, 2},
	} {
		clock.now = clock.now.Add(step.advance)
		if _, err := source.accessToken(context.Background()); err != nil {
			t.Fatal(err)
		}
		if got := endpoint.exchanges.Load(); got != step.want {
			t.Fatalf("after %s the source had exchanged %d times, want %d", step.advance, got, step.want)
		}
	}
}

func TestABurstOfCallersSharesOneExchange(t *testing.T) {
	t.Parallel()
	arrived, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	endpoint := &tokenEndpoint{respond: func(req *http.Request) tokenReply {
		once.Do(func() { close(arrived) })
		<-release
		return grantedToken("ya29.shared")(req)
	}}
	source := testTokenSource(t, endpoint, &fixedClock{now: time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)})

	const callers = 16
	tokens := make(chan string, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := source.accessToken(context.Background())
			if err != nil {
				t.Errorf("a caller failed: %v", err)
			}
			tokens <- token
		}()
	}
	<-arrived
	close(release)
	wg.Wait()
	close(tokens)
	for token := range tokens {
		if token != "ya29.shared" {
			t.Errorf("a caller got %q", token)
		}
	}
	if got := endpoint.exchanges.Load(); got != 1 {
		t.Errorf("%d callers spent %d exchanges, want 1", callers, got)
	}
}

func TestATokenRedirectIsNotFollowed(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusFound, http.StatusTemporaryRedirect} {
		endpoint := &tokenEndpoint{respond: func(*http.Request) tokenReply {
			return tokenReply{status: status, body: `{}`, location: "https://attacker.example/token"}
		}}
		source := testTokenSource(t, endpoint, &fixedClock{now: time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)})
		_, err := source.accessToken(context.Background())
		if err == nil {
			t.Fatalf("http %d: a redirected exchange produced a token", status)
		}
		if endpoint.elsewhere.Load() != 0 {
			t.Errorf("http %d: the assertion was forwarded to the redirect target", status)
		}
	}
}

func TestATokenExchangeErrorCarriesNoCredential(t *testing.T) {
	t.Parallel()
	var assertion string
	capture := func(req *http.Request) {
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("reading the exchange body: %v", err)
		}
		form, err := url.ParseQuery(string(raw))
		if err != nil {
			t.Errorf("parsing the exchange body: %v", err)
		}
		assertion = form.Get("assertion")
	}
	for name, tc := range map[string]struct {
		reply tokenReply
		fault error
		names string
	}{
		"an OAuth refusal": {
			reply: tokenReply{status: http.StatusBadRequest, body: `{"error":"invalid_grant","error_description":"Invalid JWT Signature for crm@x"}`},
			names: "http 400, invalid_grant",
		},
		"an error field that is prose": {
			reply: tokenReply{status: http.StatusUnauthorized, body: `{"error":"<html>go away crm@x</html>"}`},
			names: "http 401",
		},
		"a transport failure": {fault: errors.New("connection reset"), names: "connection reset"},
		"a 200 with no token": {reply: tokenReply{status: http.StatusOK, body: `{"token_type":"Bearer"}`}, names: "no usable access token"},
	} {
		t.Run(name, func(t *testing.T) {
			account, err := parseVertexServiceAccount(serviceAccountJSON(t, nil))
			if err != nil {
				t.Fatal(err)
			}
			transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				capture(req)
				if tc.fault != nil {
					return nil, tc.fault
				}
				return tc.reply.response(), nil
			})
			source := newVertexTokenSource(account, &http.Client{Transport: transport}, &fixedClock{now: time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)})
			_, err = source.accessToken(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.names) {
				t.Fatalf("want an error naming %q, got %v", tc.names, err)
			}
			var urlErr *url.Error
			if errors.As(err, &urlErr) {
				t.Errorf("the error wraps a *url.Error, which renders the request it failed on: %v", err)
			}
			for _, secret := range []string{assertion, "BEGIN PRIVATE KEY", "crm@x", "Invalid JWT"} {
				if secret != "" && strings.Contains(err.Error(), secret) {
					t.Errorf("the error carries %q: %v", secret, err)
				}
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
