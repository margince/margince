// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package oidctokentest is the signed-token rig the OIDC verifier's tests and
// the sign-in tests share: one RSA key behind a JWKS endpoint.
package oidctokentest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"maps"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// The rig signs every token for one audience, service account and key id.
const (
	Aud = "https://api.test/hooks/gmail/push"
	SA  = "gmail-push@margince.iam.gserviceaccount.com"
	KID = "test-key-1"
)

// Rig serves a JWKS for one RSA key and mints signed tokens against
// it. Base is the rig's single fixed instant: minted claims and the verifier
// clock both derive from it, so no test reads the real clock. certsHits
// counts how many times the /certs endpoint was actually hit, for asserting
// refresh-throttling behavior.
type Rig struct {
	key       *rsa.PrivateKey
	Base      time.Time
	Srv       *httptest.Server
	certsHits atomic.Int32
	// CertsGate, when set before the first fetch, runs at the top of the
	// /certs handler. The coalescing test uses it to hold a fetch in flight.
	CertsGate func()
}

// NewRig starts the JWKS server and closes it when the test ends.
func NewRig(t *testing.T) *Rig {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rig := &Rig{key: key, Base: time.Unix(1750000000, 0)}
	mux := http.NewServeMux()
	certs := func(w http.ResponseWriter, _ *http.Request) {
		rig.certsHits.Add(1)
		if rig.CertsGate != nil {
			rig.CertsGate()
		}
		n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{
				{"kid": KID, "kty": "RSA", "alg": "RS256", "use": "sig", "n": n, "e": e},
			},
		}); err != nil {
			t.Errorf("encode test JWKS response: %v", err)
		}
	}
	mux.HandleFunc("/certs", certs)
	// Where a Dispact install publishes its keys, so the rig's URL can stand
	// in for the install's address.
	mux.HandleFunc("/.well-known/jwks.json", certs)
	rig.Srv = httptest.NewServer(mux)
	t.Cleanup(rig.Srv.Close)
	return rig
}

// JWKSURL is where a verifier under test fetches the rig's key.
func (r *Rig) JWKSURL() string { return r.Srv.URL + "/certs" }

// CertsHitCount is how often the /certs endpoint was fetched.
func (r *Rig) CertsHitCount() int32 { return r.certsHits.Load() }

// Mint builds a signed JWT. Pass kid="" to sign without a kid header, and alg
// to override RS256. claims are merged over a default that is valid at the
// rig's fixed base instant.
func (r *Rig) Mint(t *testing.T, kid, alg string, claims map[string]any) string {
	t.Helper()
	if alg == "" {
		alg = "RS256"
	}
	header := map[string]any{"alg": alg, "typ": "JWT"}
	if kid != "" {
		header["kid"] = kid
	}
	base := map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            Aud,
		"email":          SA,
		"email_verified": true,
		"exp":            r.Base.Add(time.Hour).Unix(),
		"iat":            r.Base.Add(-time.Minute).Unix(),
	}
	maps.Copy(base, claims)
	seg := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal JWT segment: %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signingInput := seg(header) + "." + seg(base)
	h := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, r.key, crypto.SHA256, h[:])
	if err != nil {
		t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// SignedWith is a rig that signs with another key but serves this rig's JWKS,
// for a token whose signature the served key does not verify.
func (r *Rig) SignedWith(key *rsa.PrivateKey) *Rig {
	return &Rig{key: key, Base: r.Base, Srv: r.Srv}
}
