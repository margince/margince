// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The signed `state` for the connector OAuth handshake. The callback is
// session-less (SameSite=Strict means the crm_session cookie is not sent on
// the provider's cross-site redirect), so `state` is the only trustworthy
// carrier of WHO started the flow. We HMAC-sign (workspace, user, provider,
// expiry) with a server-only key: the callback recovers that tuple from a
// state it can verify it minted, sets the workspace GUC + a human principal
// from it, and persists the connection. Forgery (e.g. swapping in a victim's
// user id) fails the MAC; replay past the TTL fails the expiry; replay within
// the TTL is bounded by the provider's own single-use authorization code.

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

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// connectState is the tuple bound into a signed OAuth state parameter. Nonce
// is the CSRF binding: it must equal the SameSite=Lax nonce cookie the
// callback receives, proving the initiator and the completer are the same
// browser (the signed state alone only proves the initiator).
type connectState struct {
	Workspace ids.UUID
	User      ids.UUID
	Provider  string
	Nonce     string
	// Version names the state's own vintage, so a callback served by a newer
	// build can honour a round-trip an older one started. Absent (zero) on
	// every state minted before versioning existed.
	Version int
	// ReturnTo names the surface that started the connect, so the callback can
	// land the browser where the user actually is. A closed enum resolved by
	// landingURL, never a URL — it rides the signed payload so it cannot be
	// tampered with after the redirect leaves us.
	ReturnTo string
}

// wireState is the JSON form actually signed — ids.UUID as strings, plus the
// expiry, so the payload is self-describing and version-tolerant.
type wireState struct {
	Workspace string `json:"ws"`
	User      string `json:"u"`
	Provider  string `json:"p"`
	Nonce     string `json:"n"`
	ReturnTo  string `json:"rt,omitempty"`
	Version   int    `json:"v,omitempty"`
	Exp       int64  `json:"exp"` // unix seconds
}

// stateSigner mints and verifies signed state tokens with an HMAC key.
type stateSigner struct{ key []byte }

// newStateSigner refuses a key below the floor rather than trusting whoever
// built it to have checked.
//
// The check lived only in the caller's mount condition, so a construction site
// that forgot it produced a signer that HMACs with an empty key and verifies
// anything — the exact failure minStateKeyLen exists to prevent, arriving
// through the one path nobody was watching. A zero signer signs nothing and
// verifies nothing, so the surface refuses instead of accepting forgeries.
func newStateSigner(key []byte) stateSigner {
	if len(key) < minStateKeyLen {
		return stateSigner{}
	}
	return stateSigner{key: key}
}

// usable reports whether this signer holds a key that clears the floor.
func (s stateSigner) usable() bool { return len(s.key) >= minStateKeyLen }

// sign returns `base64url(payload).base64url(hmac(payload))`, binding the
// tuple until exp.
func (s stateSigner) sign(st connectState, exp time.Time) string {
	payload, _ := json.Marshal(wireState{ //nolint:errchkjson // string/int-only struct never errors
		Workspace: st.Workspace.String(),
		User:      st.User.String(),
		Provider:  st.Provider,
		Nonce:     st.Nonce,
		ReturnTo:  st.ReturnTo,
		Version:   st.Version,
		Exp:       exp.Unix(),
	})
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return enc + "." + base64.RawURLEncoding.EncodeToString(s.mac(enc))
}

// verify checks the signature and expiry against now and returns the bound
// tuple. Every failure mode is an error, never a partial result.
func (s stateSigner) verify(token string, now time.Time) (connectState, error) {
	enc, macPart, ok := strings.Cut(token, ".")
	if !ok {
		return connectState{}, errors.New("connector state: malformed token")
	}
	gotMAC, err := base64.RawURLEncoding.DecodeString(macPart)
	if err != nil {
		return connectState{}, fmt.Errorf("connector state: bad signature encoding: %w", err)
	}
	if subtle.ConstantTimeCompare(gotMAC, s.mac(enc)) != 1 {
		return connectState{}, errors.New("connector state: signature mismatch")
	}
	payload, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return connectState{}, fmt.Errorf("connector state: bad payload encoding: %w", err)
	}
	var w wireState
	if err := json.Unmarshal(payload, &w); err != nil {
		return connectState{}, fmt.Errorf("connector state: bad payload: %w", err)
	}
	if now.Unix() > w.Exp {
		return connectState{}, errors.New("connector state: expired")
	}
	ws, err := ids.Parse(w.Workspace)
	if err != nil {
		return connectState{}, fmt.Errorf("connector state: bad workspace id: %w", err)
	}
	user, err := ids.Parse(w.User)
	if err != nil {
		return connectState{}, fmt.Errorf("connector state: bad user id: %w", err)
	}
	return connectState{
		Workspace: ws, User: user, Provider: w.Provider,
		Nonce: w.Nonce, ReturnTo: w.ReturnTo, Version: w.Version,
	}, nil
}

func (s stateSigner) mac(enc string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(enc))
	return m.Sum(nil)
}
