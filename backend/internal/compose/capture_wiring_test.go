// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The capture registry wiring as behavior: which connectors a
// configuration yields, and when the worker's sync registry exists at
// all — the composition rules the process roles rely on at boot.

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
)

func TestCaptureRegistryComposition(t *testing.T) {
	gmailApp := GmailConfig{ClientID: "id", ClientSecret: "secret"}

	// EITHER source registers, and that is the whole rule. The registration
	// used to require the deployment's pair, which made the stored app
	// unusable rather than merely unread: the transport asks the registry
	// whether a connector exists before it will run the consent flow, so an
	// installation that set its app through Settings was sent to the declared
	// 501 with no way to connect Gmail at all.
	//
	// Driven through newCaptureRegistryWithGoogle, which is the one
	// constructor that ships. Two others used to stand in front of it — one
	// gating on the environment alone — and a case driving those proved the
	// rule of a function nothing called.
	stored := func(context.Context) (string, string, bool, error) { return "id", "secret", true, nil }

	t.Run("the deployment's pair registers the connector", func(t *testing.T) {
		assertRegisters(t, newCaptureRegistryWithGoogle(nil, nil, nil, gmailApp, CaptureConfig{}), "gmail", "gcal")
	})

	t.Run("a stored app registers it with no pair composed", func(t *testing.T) {
		assertRegisters(t, newCaptureRegistryWithGoogle(nil, nil, stored, GmailConfig{}, CaptureConfig{}), "gmail", "gcal")
	})

	t.Run("neither source carries standing imap, never gmail", func(t *testing.T) {
		reg := newCaptureRegistryWithGoogle(nil, nil, nil, GmailConfig{}, CaptureConfig{})
		names := registeredNames(reg.Connectors())
		if names["gmail"] {
			t.Fatal("gmail must be absent when neither the stored app nor the deployment's pair can supply one")
		}
		if !names["imap"] {
			t.Fatal("the standing imap connector needs no app and must always register")
		}
	})

	t.Run("connect needs more than sync", func(t *testing.T) {
		if gmailApp.Enabled() {
			t.Fatal("sync credentials alone must not enable the connect transport")
		}
		full := GmailConfig{
			ClientID: "id", ClientSecret: "secret",
			StateKey:      "0123456789abcdef0123456789abcdef",
			PublicBaseURL: "https://crm.example",
		}
		if !full.Enabled() {
			t.Fatal("a fully-configured app must enable the connect transport")
		}
	})
}

// CoreChannelProviders is the boot step's core half, and it enumerates the
// transports off a registry built with no pool and no vault. That is only
// honest while the vault-carrying and Google-app-carrying constructions add no
// channel-capable connector of their own — so the obligation is derived from
// the constructions themselves rather than restated as a list, and a connector
// that starts supplying a transport fails HERE instead of going unregistered on
// every install that has no Google app.
func TestCoreChannelProvidersEnumeratesEveryComposedCoreTransport(t *testing.T) {
	enumerated := CoreChannelProviders()
	if len(enumerated) == 0 {
		t.Fatal("this binary composed no core transport at all, so this gate would compare two empty sets")
	}
	// The richest core composition there is: both OAuth apps configured, so
	// every connector any role can register is on it.
	richest := CaptureSyncRegistry(nil, nil,
		GmailConfig{ClientID: "id", ClientSecret: "secret"},
		GraphConfig{ClientID: "id", ClientSecret: "secret"},
		CaptureConfig{}, nil).ChannelProviders()
	if !slices.Equal(enumerated, richest) {
		t.Errorf("the fully-composed registry supplies transports %v, but the boot step would register %v — "+
			"a transport the boot step cannot see is one no captured message may name",
			richest, enumerated)
	}
}

func TestWithKeyvaultWiresTheCredentialCustodian(t *testing.T) {
	s := &Server{}
	WithKeyvault(fakeVault{})(s, nil)
	if s.vault == nil {
		t.Fatal("the vault must be held for the connector-credential paths")
	}
	if s.connectorHandlers.registry == nil {
		t.Fatal("the standing connect must get a registry when none is wired yet")
	}
	// A gmail-carrying registry wired earlier must NOT be replaced.
	marker := NewCaptureRegistry(nil, nil, CaptureConfig{})
	s2 := &Server{}
	s2.connectorHandlers.registry = marker
	WithKeyvault(fakeVault{})(s2, nil)
	if s2.connectorHandlers.registry != marker {
		t.Fatal("WithKeyvault must not displace an already-wired connector registry")
	}
}

// TestWithKeyvaultWiresTheSendPreflightWithNoGoogleApp pins the fix for the
// Telegram-only wiring gap: previously the outbound send pre-flight was
// installed ONLY by WithGmailCapture, so a role with no Google app configured
// composed no pre-flight at all — including for the channel branch, which
// never depended on Gmail in the first place (NewCaptureRegistry registers
// Telegram unconditionally). WithKeyvault must now install it over whichever
// registry it just ensured exists, with no Gmail/Graph option involved.
func TestWithKeyvaultWiresTheSendPreflightWithNoGoogleApp(t *testing.T) {
	s := &Server{}
	WithKeyvault(fakeVault{})(s, nil)
	if s.send.SendAuthority == nil {
		t.Fatal("WithKeyvault must install the send pre-flight even with no Google app configured")
	}
	authority, ok := s.send.SendAuthority.(mailboxAuthority)
	if !ok {
		t.Fatalf("send pre-flight = %T, want a mailboxAuthority", s.send.SendAuthority)
	}
	if authority.grants != s.connectorHandlers.registry {
		t.Fatal("the pre-flight must read the SAME registry the connect flow writes to, not a second construction")
	}
}

// assertRegisters is the shape both reachability arms assert, so neither can
// drift into checking less than the other.
func assertRegisters(t *testing.T, reg *capture.Registry, want ...string) {
	t.Helper()
	names := registeredNames(reg.Connectors())
	for _, name := range want {
		if !names[name] {
			t.Errorf("connectors = %v, want %s registered", names, name)
		}
	}
}
