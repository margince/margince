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

// signPayload signs an arbitrary payload with the signer's key, so a test can
// craft a validly-signed token whose payload is otherwise malformed.
func signPayload(s stateSigner, payload []byte) string {
	enc := base64.RawURLEncoding.EncodeToString(payload)
	return enc + "." + base64.RawURLEncoding.EncodeToString(s.mac(enc))
}

func TestVerifyRejectsMalformedStates(t *testing.T) {
	s := newStateSigner([]byte("a-32-byte-or-longer-signing-key!!"))
	now := time.Unix(1_700_000_000, 0)
	future := now.Add(time.Hour).Unix()
	badUser, _ := json.Marshal(wireState{User: "not-a-uuid", Provider: "gmail", Exp: future})

	cases := map[string]string{
		"no separator":         "no-dot-here",
		"bad signature base64": "YWJj.@@@not-base64@@@",
		"bad payload base64":   "@@@." + base64.RawURLEncoding.EncodeToString(s.mac("@@@")),
		"non-json payload":     signPayload(s, []byte("not json at all")),
		"bad user id":          signPayload(s, badUser),
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := s.verify(tok, now); err == nil {
				t.Fatalf("verify accepted a %s token", name)
			}
		})
	}
}

func TestConnectStateRoundTrips(t *testing.T) {
	s := newStateSigner([]byte("a-32-byte-or-longer-signing-key!!"))
	user := ids.MustParse("22222222-2222-2222-2222-222222222222")
	now := time.Unix(1_700_000_000, 0)

	token := s.sign(connectState{User: user, Provider: "gmail"}, now.Add(10*time.Minute))
	got, err := s.verify(token, now)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.User != user || got.Provider != "gmail" {
		t.Errorf("round-trip = %+v, want user/gmail", got)
	}
}

func TestConnectStateRejectsTamper(t *testing.T) {
	s := newStateSigner([]byte("a-32-byte-or-longer-signing-key!!"))
	now := time.Unix(1_700_000_000, 0)
	token := s.sign(connectState{User: ids.MustParse("22222222-2222-2222-2222-222222222222"), Provider: "gmail"}, now.Add(time.Minute))

	// Flip a character in the payload half — the HMAC must no longer match.
	payload, mac, _ := strings.Cut(token, ".")
	tampered := payload[:len(payload)-1] + flip(payload[len(payload)-1:]) + "." + mac
	if _, err := s.verify(tampered, now); err == nil {
		t.Fatal("verify accepted a tampered state")
	}
}

func TestConnectStateRejectsWrongKey(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	token := newStateSigner([]byte("a-32-byte-or-longer-signing-key!!")).
		sign(connectState{User: ids.MustParse("22222222-2222-2222-2222-222222222222"), Provider: "gmail"}, now.Add(time.Minute))
	if _, err := newStateSigner([]byte("a-DIFFERENT-32-byte-signing-key!!!")).verify(token, now); err == nil {
		t.Fatal("verify accepted a state signed with a different key")
	}
}

func TestConnectStateRejectsExpired(t *testing.T) {
	s := newStateSigner([]byte("a-32-byte-or-longer-signing-key!!"))
	now := time.Unix(1_700_000_000, 0)
	token := s.sign(connectState{User: ids.MustParse("22222222-2222-2222-2222-222222222222"), Provider: "gmail"}, now.Add(time.Minute))
	if _, err := s.verify(token, now.Add(2*time.Minute)); err == nil {
		t.Fatal("verify accepted an expired state")
	}
}

func flip(s string) string {
	if s == "A" {
		return "B"
	}
	return "A"
}

func TestConnectStateCarriesReturnToThroughSignAndVerify(t *testing.T) {
	s := newStateSigner([]byte("0123456789abcdef0123456789abcdef"))
	// Fixed instant — the signer's expiry check is deterministic, so no
	// production clock seam is needed and time.Now() would only add flake.
	now := time.Unix(1_700_000_000, 0)
	token := s.sign(connectState{Provider: "gmail", Nonce: "n", ReturnTo: "settings"}, now.Add(time.Minute))
	got, err := s.verify(token, now)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.ReturnTo != "settings" {
		t.Errorf("ReturnTo = %q, want %q", got.ReturnTo, "settings")
	}
}

func TestConnectStateCarriesChainAndImportWindow(t *testing.T) {
	s := newStateSigner([]byte("0123456789abcdef0123456789abcdef"))
	now := time.Unix(1_700_000_000, 0)
	token := s.sign(connectState{Provider: "gmail", Nonce: "n", ReturnTo: "settings", Then: "gcal", WindowMonths: 6}, now.Add(time.Minute))
	got, err := s.verify(token, now)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Then != "gcal" || got.WindowMonths != 6 {
		t.Errorf("Then, WindowMonths = %q, %d; want gcal, 6", got.Then, got.WindowMonths)
	}
	if got.Provider != "gmail" || got.ReturnTo != "settings" {
		t.Errorf("Provider, ReturnTo = %q, %q; want gmail, settings", got.Provider, got.ReturnTo)
	}
}

// A connect started before the chain existed is still in flight across a
// deploy. Its state carries neither key and must land as a plain connect.
func TestConnectStateMintedWithoutChainStillVerifies(t *testing.T) {
	s := newStateSigner([]byte("0123456789abcdef0123456789abcdef"))
	now := time.Unix(1_700_000_000, 0)
	ws, user := "11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"
	legacy := `{"ws":"` + ws + `","u":"` + user + `","p":"gmail","n":"n","rt":"onboarding","exp":1700000060}`

	got, err := s.verify(signPayload(s, []byte(legacy)), now)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Then != "" || got.WindowMonths != 0 {
		t.Errorf("Then, WindowMonths = %q, %d; want empty, 0", got.Then, got.WindowMonths)
	}
	if got.Provider != "gmail" || got.ReturnTo != "onboarding" {
		t.Errorf("legacy state decoded as %+v", got)
	}
}

func TestConnectStateWithoutChainSignsNoChainKeys(t *testing.T) {
	s := newStateSigner([]byte("0123456789abcdef0123456789abcdef"))
	token := s.sign(connectState{Provider: "gcal", Nonce: "n"}, time.Unix(1_700_000_060, 0))
	enc, _, _ := strings.Cut(token, ".")
	payload, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(payload, &keys); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	for _, k := range []string{"th", "iw"} {
		if _, ok := keys[k]; ok {
			t.Errorf("payload carries %q for a state that ends the chain: %s", k, payload)
		}
	}
}

// A replica one release older requires the `ws` field and rejects a state
// without it. A state signed here carries one that parses as an id.
func TestASignedStateCarriesTheWorkspaceAnOlderReplicaRequires(t *testing.T) {
	company := ids.MustParse("33333333-3333-3333-3333-333333333333")
	s := newStateSigner([]byte("a-32-byte-or-longer-signing-key!!"))
	s.company = func() ids.UUID { return company }

	token := s.sign(connectState{User: ids.NewV7(), Provider: "gmail"}, time.Unix(1_700_000_060, 0))

	payload, err := base64.RawURLEncoding.DecodeString(strings.SplitN(token, ".", 2)[0])
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Workspace string `json:"ws"`
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if got, err := ids.Parse(wire.Workspace); err != nil || got != company {
		t.Fatalf("ws = %q (%v), want the company %s", wire.Workspace, err, company)
	}
}
