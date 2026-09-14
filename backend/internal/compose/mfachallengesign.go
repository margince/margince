// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The signed second-factor challenge. A password login that lands on a member
// with an active factor hands back one of these instead of a session; the
// member returns it to /auth/mfa with a code. HMAC mechanics mirror the OIDC
// login-state signer beside it — the same deployment HMAC key may sign both, so
// a token-type tag domain-separates them the way loginStateTyp does. Each token
// also carries a random nonce (jti): the signature makes a challenge
// unforgeable, and the nonce — spent by identity in the session-minting
// transaction — makes it single-use.

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const mfaChallengeTyp = "mfa-challenge"

// mfaChallengeNonceLen sizes the jti: 16 random bytes, the same order of
// entropy as a session token's, so spent-nonce collisions are never how two
// challenges become one.
const mfaChallengeNonceLen = 16

type wireMFAChallenge struct {
	Typ string `json:"t"`
	Sub string `json:"sub"`
	Jti string `json:"jti"`
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

// Sign mints a challenge binding the member for ttl, with a fresh nonce. The
// error path exists for the nonce alone: a process whose crypto/rand fails must
// refuse to mint a challenge rather than issue one that is not single-use.
func (s mfaChallengeSigner) Sign(userID string, ttl time.Duration) (string, error) {
	nonce := make([]byte, mfaChallengeNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("mfa challenge: minting nonce: %w", err)
	}
	payload, _ := json.Marshal(wireMFAChallenge{ //nolint:errchkjson // string/int-only struct never errors
		Typ: mfaChallengeTyp, Sub: userID,
		Jti: base64.RawURLEncoding.EncodeToString(nonce),
		Exp: s.now().Add(ttl).Unix(),
	})
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return enc + "." + base64.RawURLEncoding.EncodeToString(s.mac(enc)), nil
}

// Verify checks the signature, type and expiry, and returns the member the
// challenge binds together with its nonce. A token minted without a nonce is
// refused outright — it predates the single-use ledger and could be replayed
// for its whole TTL.
func (s mfaChallengeSigner) Verify(token string) (string, string, error) {
	enc, macPart, ok := strings.Cut(token, ".")
	if !ok {
		return "", "", errors.New("mfa challenge: malformed token")
	}
	gotMAC, err := base64.RawURLEncoding.DecodeString(macPart)
	if err != nil {
		return "", "", fmt.Errorf("mfa challenge: bad signature encoding: %w", err)
	}
	if subtle.ConstantTimeCompare(gotMAC, s.mac(enc)) != 1 {
		return "", "", errors.New("mfa challenge: signature mismatch")
	}
	payload, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", "", fmt.Errorf("mfa challenge: bad payload encoding: %w", err)
	}
	var w wireMFAChallenge
	if err := json.Unmarshal(payload, &w); err != nil {
		return "", "", fmt.Errorf("mfa challenge: bad payload: %w", err)
	}
	if w.Typ != mfaChallengeTyp {
		return "", "", errors.New("mfa challenge: wrong token type")
	}
	if w.Jti == "" {
		return "", "", errors.New("mfa challenge: missing nonce")
	}
	if s.now().Unix() > w.Exp {
		return "", "", errors.New("mfa challenge: expired")
	}
	return w.Sub, w.Jti, nil
}

func (s mfaChallengeSigner) mac(enc string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(enc))
	return m.Sum(nil)
}

// WithMFAChallengeSigner arms the second-factor login challenge with the
// deployment's HMAC key. Without it a login that would challenge fails closed
// rather than mint an unsigned token; the same key the OAuth state flows use is
// reused, domain-separated by token type, so no second secret is required.
//
// A short or absent key leaves the signer unarmed AND — via armMFAEnrolment —
// keeps TOTP enrolment off even where a vault exists, reported at ERROR when it
// does. The two halves must travel together: enrolment with no signer lets a
// member confirm a factor whose login challenge can never be answered, which is
// a lockout the moment they next sign in.
func WithMFAChallengeSigner(key string) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		signer := newMFAChallengeSigner([]byte(key))
		if signer.key == nil {
			s.mfaSignerUnusable = true
		} else {
			s.mfaSigner = signer
			s.authHandlers = s.WithMFAChallengeSigner(signer)
		}
		s.armMFAEnrolment()
	}
}

// armMFAEnrolment is the ONE place TOTP enrolment turns on, called by whichever
// of WithMFAChallengeSigner and WithKeyvault runs second (options compose in
// any order): the vault seals the secret and the signer makes the resulting
// login challenge answerable, and neither half alone is a working factor. While
// either is missing the enrolment routes keep their not-composed refusal, the
// require-MFA policy cannot confine (the identity service reads its own nil
// vault for both), and — when the missing half is the KEY beside a present
// vault — the operator hears about it at ERROR, because that state was almost
// certainly meant to be MFA and would otherwise fail only at a member's login.
func (s *Server) armMFAEnrolment() {
	if s.vault == nil {
		return
	}
	if s.mfaSigner == nil {
		if s.mfaSignerUnusable {
			logger := s.log
			if logger == nil {
				// A Server assembled outside newServer (wiring tests) carries no
				// logger; the misconfiguration still deserves a line somewhere.
				logger = slog.Default()
			}
			logger.Error("multi-factor enrolment is OFF although a key vault is configured: " +
				"the challenge-signing key is missing or shorter than 32 bytes, so a second-factor login could never complete. " +
				"Set --connector-state-key (MARGINCE_CONNECTOR_STATE_KEY) to at least 32 bytes to enable MFA")
		}
		return
	}
	s.authHandlers = s.WithMFAVault(s.vault)
}
