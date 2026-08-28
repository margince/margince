// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// valid is the smallest endpoint Validate admits; each refusal case below is
// this one with a single field spoiled, so a test names exactly the field it is
// about.
func valid() extension.InboundEndpoint {
	return extension.InboundEndpoint{
		Slug:    "capture",
		Secret:  "inbound",
		MaxBody: 64 << 10,
		Rate: extension.InboundRate{
			PerIP:       extension.Rate{Limit: 60, Window: time.Minute},
			PerEndpoint: extension.Rate{Limit: 120, Window: time.Minute},
		},
		Skew: 5 * time.Minute,
		Handle: func(context.Context, extension.Runtime, extension.InboundRequest) (extension.InboundOutcome, error) {
			return extension.InboundAccepted, nil
		},
	}
}

func TestInboundEndpointValidateAdmitsAWholeDeclaration(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("a complete declaration was refused: %v", err)
	}
}

func TestInboundEndpointValidateRefusals(t *testing.T) {
	tests := []struct {
		name  string
		spoil func(*extension.InboundEndpoint)
		// want is a phrase the refusal must carry, so a test fails when the
		// message stops saying what the author has to fix.
		want string
	}{
		{"empty slug", func(e *extension.InboundEndpoint) { e.Slug = "" }, "empty slug"},
		{"blank slug", func(e *extension.InboundEndpoint) { e.Slug = "   " }, "empty slug"},
		{"slug over the cap", func(e *extension.InboundEndpoint) { e.Slug = strings.Repeat("a", 33) }, "unmetered"},
		{"slug with an underscore", func(e *extension.InboundEndpoint) { e.Slug = "cap_ture" }, "is not a slug"},
		{"slug in upper case", func(e *extension.InboundEndpoint) { e.Slug = "Capture" }, "is not a slug"},
		{"slug with a double hyphen", func(e *extension.InboundEndpoint) { e.Slug = "cap--ture" }, "is not a slug"},
		{"no secret", func(e *extension.InboundEndpoint) { e.Secret = "" }, "names no secret"},
		{"blank secret", func(e *extension.InboundEndpoint) { e.Secret = "  " }, "names no secret"},
		{"no body cap", func(e *extension.InboundEndpoint) { e.MaxBody = 0 }, "no default"},
		{"negative body cap", func(e *extension.InboundEndpoint) { e.MaxBody = -1 }, "no default"},
		{"body cap over the ceiling", func(e *extension.InboundEndpoint) { e.MaxBody = extension.MaxInboundBody + 1 }, "over the"},
		{"no skew", func(e *extension.InboundEndpoint) { e.Skew = 0 }, "replayable indefinitely"},
		{"negative skew", func(e *extension.InboundEndpoint) { e.Skew = -time.Second }, "replayable indefinitely"},
		{"skew over the ceiling", func(e *extension.InboundEndpoint) { e.Skew = extension.MaxInboundSkew + time.Second }, "over the"},
		{"no handler", func(e *extension.InboundEndpoint) { e.Handle = nil }, "declares no handler"},
		{"no per-IP limit", func(e *extension.InboundEndpoint) { e.Rate.PerIP.Limit = 0 }, "per-IP allowance"},
		{"no per-IP window", func(e *extension.InboundEndpoint) { e.Rate.PerIP.Window = 0 }, "per-IP allowance over no window"},
		{"no per-endpoint limit", func(e *extension.InboundEndpoint) { e.Rate.PerEndpoint.Limit = 0 }, "per-endpoint allowance"},
		{"no per-endpoint window", func(e *extension.InboundEndpoint) { e.Rate.PerEndpoint.Window = 0 }, "per-endpoint allowance over no window"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := valid()
			tc.spoil(&e)
			err := e.Validate()
			if err == nil {
				t.Fatalf("Validate admitted %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate said %q, which does not carry %q — the message must say what to fix", err, tc.want)
			}
		})
	}
}

// The signed material is the one thing a sender and a verifier must spell
// identically, so it is pinned byte for byte rather than described. A reader
// writing a sender against this product copies it from here.
func TestInboundRequestSignedPayloadIsPinned(t *testing.T) {
	req := extension.InboundRequest{
		Slug:      "capture",
		Ref:       "mB1e7Zq",
		Timestamp: time.Unix(1700000000, 0),
		Nonce:     "0f1e2d3c",
		Body:      []byte(`{"hello":"world"}`),
	}
	const want = "margince-inbound-v1\ncapture\nmB1e7Zq\n1700000000\n0f1e2d3c\n" + `{"hello":"world"}`
	if got := string(req.SignedPayload()); got != want {
		t.Fatalf("SignedPayload() = %q, want %q", got, want)
	}
}

// A sub-second timestamp signs the same seconds value it was sent as: the
// header carries seconds, so a verifier that used the full time.Time would
// compute a MAC the sender never could.
func TestInboundRequestSignedPayloadUsesWholeSeconds(t *testing.T) {
	req := extension.InboundRequest{
		Timestamp: time.Unix(1700000000, 500_000_000),
		Nonce:     "ab",
		Body:      nil,
	}
	if got := string(req.SignedPayload()); got != "margince-inbound-v1\n\n\n1700000000\nab\n" {
		t.Fatalf("SignedPayload() = %q, want the whole-second form", got)
	}
}

// An empty body is a legal signed request, and the two separators must still be
// present — otherwise a body of "x" and a nonce of "x" would sign alike.
func TestInboundRequestSignedPayloadKeepsBothSeparators(t *testing.T) {
	a := extension.InboundRequest{Timestamp: time.Unix(1, 0), Nonce: "x", Body: nil}
	b := extension.InboundRequest{Timestamp: time.Unix(1, 0), Nonce: "", Body: []byte("x")}
	if string(a.SignedPayload()) == string(b.SignedPayload()) {
		t.Fatalf("a nonce and a body signed alike: %q", a.SignedPayload())
	}
}

// The replay key and the body must not be separable by a caller. Under the
// earlier `<ts>.<nonce>.<body>` spelling they were: every separator inside the
// body marked a boundary the same bytes could be re-split at, yielding a
// DIFFERENT nonce over IDENTICAL signing material. One captured request then
// replayed once per separator in its own body — each replay landing a fresh row
// past a uniqueness index that never saw a collision, and none of it needing
// the secret.
//
// The alternative splits are DERIVED from the message rather than listed, so
// the case cannot be satisfied by a spelling that happens to escape the three
// examples somebody thought of.
func TestSigningPayloadCannotBeReSplitBetweenNonceAndBody(t *testing.T) {
	at := time.Unix(1_787_910_000, 0)
	const nonce = "a1b2"
	body := []byte(`{"m":"hi. there. and. again"}`)
	captured := extension.SigningPayload(extension.ScopeInbound, "capture", "r1", at, nonce, body)

	// Every way of moving the nonce/body boundary rightwards past a separator
	// the body itself contains.
	tried := 0
	for i, c := range string(body) {
		if c != '.' && c != '\n' {
			continue
		}
		tried++
		movedNonce := nonce + string(c) + string(body[:i])
		moved := extension.SigningPayload(extension.ScopeInbound, "capture", "r1", at, movedNonce, body[i+1:])
		if string(moved) == string(captured) {
			t.Errorf("nonce %q over the remaining body signs the same bytes as the captured request — "+
				"one signature admits a second, differently-keyed row", movedNonce)
		}
	}
	if tried == 0 {
		t.Fatal("the body carries no separator, so this test asserted nothing about re-splitting")
	}
}

// And the grammar is what keeps that property true rather than accidental: a
// nonce free to contain the separator could absorb the head of the body.
func TestValidInboundNonce(t *testing.T) {
	tests := []struct {
		nonce string
		want  bool
	}{
		{"a1b2", true},
		{"DEADBEEF", true},
		{strings.Repeat("a", extension.MaxInboundNonce), true},
		{"", false},
		{strings.Repeat("a", extension.MaxInboundNonce+1), false},
		{"a1b2.more", false},
		{"a1b2\nmore", false},
		{"nothex", false},
		{"a1 b2", false},
		{"a1b2-", false},
	}
	for _, tc := range tests {
		if got := extension.ValidInboundNonce(tc.nonce); got != tc.want {
			t.Errorf("ValidInboundNonce(%q) = %v, want %v", tc.nonce, got, tc.want)
		}
	}
}

// A signature is minted for ONE exchange. The same secret over the same message
// in the other direction must produce different material, or a party trusted to
// receive can relay what it was sent back at the sender's own edge.
func TestSigningPayloadSeparatesTheTwoDirections(t *testing.T) {
	at := time.Unix(1_787_910_000, 0)
	body := []byte(`{"m":"hi"}`)
	in := extension.SigningPayload(extension.ScopeInbound, "capture", "r1", at, "a1b2", body)
	out := extension.SigningPayload(extension.ScopeOutbound, "capture", "r1", at, "a1b2", body)
	if string(in) == string(out) {
		t.Fatal("an outbound message signs the same material as an inbound one — " +
			"every message this installation sends is a valid message to itself")
	}
}

// A request captured on one endpoint must not be valid on another. The URL
// makes that distinction and the signature has to make it too.
func TestSigningPayloadBindsTheEndpointAddressed(t *testing.T) {
	at := time.Unix(1_787_910_000, 0)
	body := []byte(`{"m":"hi"}`)
	base := extension.SigningPayload(extension.ScopeInbound, "capture", "r1", at, "a1b2", body)
	for _, other := range []struct{ slug, ref string }{
		{"capture", "r2"},
		{"receive", "r1"},
		{"capture", ""},
	} {
		if string(extension.SigningPayload(extension.ScopeInbound, other.slug, other.ref, at, "a1b2", body)) == string(base) {
			t.Errorf("slug %q ref %q signs what slug %q ref %q signs — a request captured on one "+
				"endpoint is valid on the other", other.slug, other.ref, "capture", "r1")
		}
	}
}
