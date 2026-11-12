// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package googlejwt

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"
)

func testAccount(t *testing.T) (ServiceAccount, *rsa.PrivateKey) {
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
	account, err := ParseServiceAccount(string(file))
	if err != nil {
		t.Fatalf("a well-formed key file was refused: %v", err)
	}
	return account, key
}

// claimsOf verifies the signature and returns the decoded claim set.
func claimsOf(t *testing.T, jwt string, key *rsa.PrivateKey) map[string]any {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("the assertion has %d segments, want 3", len(parts))
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatalf("the signature does not verify: %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		t.Fatalf("the claims segment is not JSON: %v", err)
	}
	return claims
}

func TestASubjectIsSignedIntoTheAssertionAndNoSubjectLeavesTheClaimOut(t *testing.T) {
	account, key := testAccount(t)
	issued := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)

	delegated, err := account.Assertion(Claims{Scope: "s", Subject: "team@example.com", IssuedAt: issued, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	claims := claimsOf(t, delegated, key)
	if claims["sub"] != "team@example.com" || claims["aud"] != TokenURI || claims["scope"] != "s" {
		t.Errorf("delegated claims = %v, want sub, the pinned aud and the scope", claims)
	}
	if claims["exp"] != float64(issued.Add(time.Hour).Unix()) {
		t.Errorf("exp = %v, want an hour after iat", claims["exp"])
	}

	own, err := account.Assertion(Claims{Scope: "s", IssuedAt: issued, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if _, present := claimsOf(t, own, key)["sub"]; present {
		t.Error("an assertion with no subject carries a sub claim")
	}
}

func TestAKeyNamingAnotherTokenEndpointIsRefused(t *testing.T) {
	_, err := ParseServiceAccount(`{"type":"service_account","client_email":"a@b","token_uri":"https://attacker.example/token"}`)
	if !errors.Is(err, ErrInvalidKey) || !strings.Contains(err.Error(), "token_uri") {
		t.Fatalf("err = %v, want a refusal naming token_uri", err)
	}
	if strings.Contains(err.Error(), "attacker.example") {
		t.Errorf("the refusal echoes the value it was given: %v", err)
	}
}

func TestAnAccountWithNoParsedKeySignsNothing(t *testing.T) {
	if _, err := (ServiceAccount{ClientEmail: "a@b"}).Assertion(Claims{}); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("err = %v, want ErrInvalidKey", err)
	}
}
