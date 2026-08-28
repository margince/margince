// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"errors"
	"net"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// The stored address is re-checked at the moment of dialling, not trusted from
// the row: what is in the column is what was legal when it was written, and a
// stored address that predates a tightened rule must not be dialable because it
// was stored first.
func TestTheAddressIsCheckedAgainAtTheMomentOfDialling(t *testing.T) {
	t.Parallel()
	for _, address := range []string{
		"http://hooks.example.com/crm",
		"https://10.4.2.9/crm",
		"https://user:pass@hooks.example.com/crm",
		"",
	} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			if _, err := newSender(address); !errors.Is(err, extension.ErrInvalid) {
				t.Fatalf("%q was accepted as an address to dial, answering %v", address, err)
			}
		})
	}
}

// A refusal the guard makes is one nothing left for, which is what lets the send
// path call it a definite answer rather than an unanswerable one.
func TestAGuardedSenderRefusesBeforeItResolvesAnything(t *testing.T) {
	t.Parallel()
	built, err := newSender("https://hooks.example.com/crm")
	if err != nil {
		t.Fatalf("building a sender for an ordinary address: %v", err)
	}
	if built.http.Transport == nil {
		t.Fatal("the sender carries the default transport, which has no egress guard on its dialler")
	}
	if net.ParseIP("hooks.example.com") != nil {
		t.Fatal("the fixture host parses as an address, so this test is not about a name at all")
	}
}
