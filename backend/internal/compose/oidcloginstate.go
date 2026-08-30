// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Signed state for the Google-sign-in login flow. Unlike connectorstate.go's
// connectState (bound to an already-known workspace+user), this flow starts
// with no session at all, so the state carries only what a pre-auth request
// can supply: which provider, a CSRF nonce, and the PKCE code_verifier — the
// verifier has nowhere server-side to live between /start and /callback since
// there is no user row yet to key a record on, so it rides the signed,
// HttpOnly cookie instead. HMAC mechanics mirror stateSigner deliberately;
// the wire shape does not, because the bound tuple is genuinely different.

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type loginState struct {
	Provider     string
	Nonce        string
	CodeVerifier string
}

type wireLoginState struct {
	P   string `json:"p"`
	N   string `json:"n"`
	CV  string `json:"cv"`
	Exp int64  `json:"exp"`
}

type loginStateSigner struct{ key []byte }

func newLoginStateSigner(key []byte) loginStateSigner {
	if len(key) < minStateKeyLen {
		return loginStateSigner{}
	}
	return loginStateSigner{key: key}
}

func (s loginStateSigner) sign(st loginState, exp time.Time) string {
	payload, _ := json.Marshal(wireLoginState{ //nolint:errchkjson // string/int-only struct never errors
		P: st.Provider, N: st.Nonce, CV: st.CodeVerifier, Exp: exp.Unix(),
	})
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return enc + "." + base64.RawURLEncoding.EncodeToString(s.mac(enc))
}

func (s loginStateSigner) verify(token string, now time.Time) (loginState, error) {
	enc, macPart, ok := strings.Cut(token, ".")
	if !ok {
		return loginState{}, errors.New("oidc login state: malformed token")
	}
	gotMAC, err := base64.RawURLEncoding.DecodeString(macPart)
	if err != nil {
		return loginState{}, fmt.Errorf("oidc login state: bad signature encoding: %w", err)
	}
	if subtle.ConstantTimeCompare(gotMAC, s.mac(enc)) != 1 {
		return loginState{}, errors.New("oidc login state: signature mismatch")
	}
	payload, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return loginState{}, fmt.Errorf("oidc login state: bad payload encoding: %w", err)
	}
	var w wireLoginState
	if err := json.Unmarshal(payload, &w); err != nil {
		return loginState{}, fmt.Errorf("oidc login state: bad payload: %w", err)
	}
	if now.Unix() > w.Exp {
		return loginState{}, errors.New("oidc login state: expired")
	}
	return loginState{Provider: w.P, Nonce: w.N, CodeVerifier: w.CV}, nil
}

func (s loginStateSigner) mac(enc string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(enc))
	return m.Sum(nil)
}
