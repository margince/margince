// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestARegistrableAddressIsHttpsAndNamesAHost(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"empty":                                "  ",
		"plain http":                           "http://example.com/hook",
		"no scheme at all":                     "example.com/hook",
		"an address, not a host":               "https://10.0.0.5/hook",
		"a loopback address":                   "https://127.0.0.1/hook",
		"a bracketed IPv6 literal":             "https://[::1]/hook",
		"a bracketed IPv6 literal with a port": "https://[::1]:8443/hook",
		"a port with no host at all":           "https://:443/hook",
		"credentials in the URL":               "https://user:pass@example.com/hook",
		"a fragment":                           "https://example.com/hook#part",
		"over the length cap":                  "https://example.com/" + strings.Repeat("a", maxURLLength),
		"not a URL at all":                     "https://exa mple.com/\x7f",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := registrableURL(raw)
			if !errors.Is(err, extension.ErrInvalid) {
				t.Fatalf("%q was accepted as %q (err %v)", raw, got, err)
			}
		})
	}
}

func TestARegistrableAddressKeepsItsPathAndQuery(t *testing.T) {
	t.Parallel()
	// Both are part of WHERE this connector posts, unlike a fragment, which is
	// never transmitted: dropping either would silently send somewhere else.
	const address = "https://example.com/hooks/crm?channel=sales"
	got, err := registrableURL("\t" + address + "\n")
	if err != nil {
		t.Fatalf("a plain https address was refused: %v", err)
	}
	if got != address {
		t.Fatalf("the checked form is %q, want %q", got, address)
	}
}

func TestARegistrableAddressMayNameAPort(t *testing.T) {
	t.Parallel()
	// The host check drops the port before asking whether the host is an
	// address literal; without that, every host:port reads as one and no real
	// deployment on a non-default port could be registered.
	const address = "https://example.com:8443/hook"
	got, err := registrableURL(address)
	if err != nil {
		t.Fatalf("a host with a port was refused: %v", err)
	}
	if got != address {
		t.Fatalf("the checked form is %q, want %q", got, address)
	}
}
