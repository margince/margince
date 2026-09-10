// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The signed second-factor challenge. A password login that lands on a member
// with an active factor hands back one of these instead of a session; the
// member returns it to /auth/mfa with a code. HMAC mechanics mirror the OIDC
// login-state signer beside it — the same deployment HMAC key may sign both, so
// a token-type tag domain-separates them the way loginStateTyp does.

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

const mfaChallengeTyp = "mfa-challenge"

type wireMFAChallenge struct {
	Typ string `json:"t"`
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
}

type mfaChallengeSigner struct {
	key []byte
	now func() time.Time
}

// newMFAChallengeSigner returns a signer, or a keyless zero value when the key
// is too short — the same fail-closed shape newLoginStateSigner has, so a
// deployment that never configured a key mints nothing that would verify.
func newMFAChallengeSigner(key []byte) mfaChallengeSigner {
	if len(key) < minStateKeyLen {
		return mfaChallengeSigner{}
	}
	return mfaChallengeSigner{key: key, now: time.Now}
}

func (s mfaChallengeSigner) Sign(userID string, ttl time.Duration) string {
	payload, _ := json.Marshal(wireMFAChallenge{ //nolint:errchkjson // string/int-only struct never errors
		Typ: mfaChallengeTyp, Sub: userID, Exp: s.now().Add(ttl).Unix(),
	})
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return enc + "." + base64.RawURLEncoding.EncodeToString(s.mac(enc))
}

func (s mfaChallengeSigner) Verify(token string) (string, error) {
	enc, macPart, ok := strings.Cut(token, ".")
	if !ok {
		return "", errors.New("mfa challenge: malformed token")
	}
	gotMAC, err := base64.RawURLEncoding.DecodeString(macPart)
	if err != nil {
		return "", fmt.Errorf("mfa challenge: bad signature encoding: %w", err)
	}
	if subtle.ConstantTimeCompare(gotMAC, s.mac(enc)) != 1 {
		return "", errors.New("mfa challenge: signature mismatch")
	}
	payload, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", fmt.Errorf("mfa challenge: bad payload encoding: %w", err)
	}
	var w wireMFAChallenge
	if err := json.Unmarshal(payload, &w); err != nil {
		return "", fmt.Errorf("mfa challenge: bad payload: %w", err)
	}
	if w.Typ != mfaChallengeTyp {
		return "", errors.New("mfa challenge: wrong token type")
	}
	if s.now().Unix() > w.Exp {
		return "", errors.New("mfa challenge: expired")
	}
	return w.Sub, nil
}

func (s mfaChallengeSigner) mac(enc string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(enc))
	return m.Sum(nil)
}
