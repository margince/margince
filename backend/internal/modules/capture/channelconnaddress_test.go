// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// addressCheck is a provider that only answers whether it needs an address.
type addressCheck struct{ needsAddress bool }

func (addressCheck) ValidateToken(string) error { return nil }
func (addressCheck) Confirm(context.Context, string, string) (ChannelBot, error) {
	return ChannelBot{}, nil
}
func (c addressCheck) NeedsAddress() bool { return c.needsAddress }
func (addressCheck) FixedIdentity() bool  { return false }

func TestConnectAddressReadsABareHostAsHTTPS(t *testing.T) {
	selfHosted := addressCheck{needsAddress: true}
	for _, c := range []struct{ raw, want string }{
		{"chat.example.com", "https://chat.example.com"},
		{" chat.example.com/ ", "https://chat.example.com"},
		{"chat.example.com:8443/team", "https://chat.example.com:8443/team"},
		{"https://chat.example.com", "https://chat.example.com"},
		{"http://chat.internal", "http://chat.internal"},
		{"httpchat.example.com", "https://httpchat.example.com"},
		{"localhost:8065", "https://localhost:8065"},
	} {
		got, err := connectAddress(selfHosted, c.raw)
		if err != nil || got != c.want {
			t.Errorf("connectAddress(%q) = %q, %v; want %q", c.raw, got, err, c.want)
		}
	}
}

func TestConnectAddressRefusesWhatNamesNoWebHost(t *testing.T) {
	selfHosted := addressCheck{needsAddress: true}
	for _, raw := range []string{"", "   ", "ftp://chat.example.com", "https://", "https:"} {
		if _, err := connectAddress(selfHosted, raw); !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("connectAddress(%q) err = %v; want ErrInvalidArgument", raw, err)
		}
	}
	if _, err := connectAddress(addressCheck{}, "chat.example.com"); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Errorf("a provider with a fixed API took an address: err = %v", err)
	}
}
