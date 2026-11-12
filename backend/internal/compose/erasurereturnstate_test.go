// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var returnLinkTestKey = []byte("0123456789abcdef0123456789abcdef")

func signedReturnLink(t *testing.T, signer returnLinkSigner, exp time.Time) (returnRequest, string) {
	t.Helper()
	request := returnRequest{
		Slug: "dana", Name: "Wieder Da", Email: "zurueck@example.test", PurposeID: ids.NewV7(),
		PolicyVersion: "pp-1", Wording: "We may contact you.", ErasedAt: time.UnixMicro(1_800_000_000_123_456),
	}
	token, err := signer.sign(request, exp)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return request, token
}

func TestAReturnLinkCarriesTheRequestAndTheErasure(t *testing.T) {
	t.Parallel()
	signer := newReturnLinkSigner(returnLinkTestKey)
	now := time.Unix(1_900_000_000, 0)
	want, token := signedReturnLink(t, signer, now.Add(returnLinkLifetime))

	got, err := signer.verify(token, now)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got != want {
		t.Fatalf("the link carried %+v, want %+v", got, want)
	}
}

func TestAReturnLinkIsRefusedOnceExpiredForgedOrForeign(t *testing.T) {
	t.Parallel()
	signer := newReturnLinkSigner(returnLinkTestKey)
	now := time.Unix(1_900_000_000, 0)
	_, token := signedReturnLink(t, signer, now.Add(returnLinkLifetime))
	payload, _, _ := strings.Cut(token, ".")
	foreign := newLoginStateSigner(returnLinkTestKey).sign(loginState{Provider: "google", Nonce: "n"}, now.Add(time.Hour))
	retyped := signedPayload(t, signer, wireReturnRequest{Typ: "oidc-login", P: ids.NewV7().String(), Exp: now.Add(time.Hour).Unix()})

	for name, verify := range map[string]func() error{
		"expired": func() error { _, err := signer.verify(token, now.Add(returnLinkLifetime+time.Second)); return err },
		"another key": func() error {
			_, err := newReturnLinkSigner([]byte(strings.Repeat("k", 32))).verify(token, now)
			return err
		},
		"no key":            func() error { _, err := newReturnLinkSigner(nil).verify(token, now); return err },
		"payload swapped":   func() error { _, err := signer.verify(payload+"x."+strings.SplitN(token, ".", 2)[1], now); return err },
		"sign-in state":     func() error { _, err := signer.verify(foreign, now); return err },
		"no signature part": func() error { _, err := signer.verify(payload, now); return err },
		"other type":        func() error { _, err := signer.verify(retyped, now); return err },
	} {
		if verify() == nil {
			t.Errorf("a %s link was accepted", name)
		}
	}
	if _, err := newReturnLinkSigner(nil).sign(returnRequest{}, now); err == nil {
		t.Error("an installation with no root key minted a link")
	}
}

// signedPayload signs a wire payload as it stands, so a test can vary one field.
func signedPayload(t *testing.T, signer returnLinkSigner, wire wireReturnRequest) string {
	t.Helper()
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString(raw)
	return enc + "." + base64.RawURLEncoding.EncodeToString(signer.mac(enc))
}
