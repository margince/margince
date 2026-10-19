// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailer

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/outbound"
	"github.com/margince/margince/backend/internal/shared/testsmtp"
)

func startFakeRelay(t *testing.T, reject bool) *testsmtp.Relay { return testsmtp.Start(t, reject) }

func TestSendWalksTheFullExchangeOnALoopbackRelay(t *testing.T) {
	relay := startFakeRelay(t, false)
	host, port := relay.Address(t)
	s := SMTP{Host: host, Port: port, FromAddress: "crm@example.test"}

	err := s.Send(context.Background(), "ada@example.test", "Reset your password", "the link body")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	got := relay.Received()
	if !strings.Contains(got, "To: ada@example.test") ||
		!strings.Contains(got, "Subject: Reset your password") ||
		!strings.Contains(got, "the link body") {
		t.Fatalf("relay received %q, want headers + body", got)
	}
}

func TestSendRefusesHeaderInjection(t *testing.T) {
	s := SMTP{Host: "127.0.0.1", Port: 25, FromAddress: "crm@example.test"}
	if err := s.Send(context.Background(), "a@b.test\r\nBcc: evil@x.test", "s", "b"); err == nil {
		t.Fatal("a CRLF recipient was accepted (header injection)")
	}
	if err := s.Send(context.Background(), "a@b.test", "s\r\nX-Evil: 1", "b"); err == nil {
		t.Fatal("a CRLF subject was accepted (header injection)")
	}
}

func TestSendSurfacesARefusedRecipient(t *testing.T) {
	relay := startFakeRelay(t, true)
	host, port := relay.Address(t)
	s := SMTP{Host: host, Port: port, FromAddress: "crm@example.test"}

	err := s.Send(context.Background(), "nobody@example.test", "s", "b")
	if err == nil || !strings.Contains(err.Error(), "refused the recipient") {
		t.Fatalf("err = %v, want the refused-recipient wrap", err)
	}
}

func TestSendRefusesAnUnreachableRelay(t *testing.T) {
	// A closed port answers immediately — no timeout flakiness.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener addr %T is not TCP", ln.Addr())
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	s := SMTP{Host: "127.0.0.1", Port: tcp.Port, FromAddress: "crm@example.test"}
	if err := s.Send(context.Background(), "a@b.test", "s", "b"); err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("err = %v, want the unreachable wrap", err)
	}
}

func TestIsLoopbackRecognizesLocalRelaysOnly(t *testing.T) {
	for host, want := range map[string]bool{
		"localhost":        true,
		"127.0.0.1":        true,
		"::1":              true,
		"smtp.example.com": false,
		"10.0.0.5":         false,
	} {
		if got := isLoopback(host); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", host, got, want)
		}
	}
}

// TestTheGreetingNamesTheSenderRatherThanLocalhost is what a relay operator
// reads to attribute the mail they are carrying.
//
// Nothing called Hello, so net/smtp announced `localhost` — and every
// installation announced it identically. That is worse than anonymous: it is a
// claim, and a false one, since the mail did not come from the relay's own
// machine.
func TestTheGreetingNamesTheSenderRatherThanLocalhost(t *testing.T) {
	cases := []struct {
		name string
		from string
		want string
	}{
		{
			name: "the sender's own domain, which the relay sees in MAIL FROM anyway",
			from: "notifications@acme.example",
			want: "acme.example",
		},
		{
			// A quoted local part may hold an `@` of its own, and the domain is
			// what follows the LAST one.
			name: "a quoted local part does not move the domain",
			from: `"bounce@internal"@acme.example`,
			want: "acme.example",
		},
		{
			name: "a trailing space is not part of the name",
			from: "notifications@acme.example ",
			want: "acme.example",
		},
		{
			name: "an address ending at the at-sign announces the product",
			from: "postmaster@",
			want: outbound.MailProduct,
		},
		{
			// Nothing to announce from, so the product token stands in — still
			// not a claim about which machine this is.
			name: "a sender with no domain falls back to the product",
			from: "postmaster",
			want: outbound.MailProduct,
		},
		{
			name: "an unconfigured sender still announces something",
			from: "",
			want: outbound.MailProduct,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The exact value, which is what guards the regression: no `want`
			// here is "localhost", so pinning the answer pins that too. A
			// second assertion against the old default could never fire.
			if got := (SMTP{FromAddress: tc.from}).helloName(); got != tc.want {
				t.Errorf("helloName() = %q, want %q", got, tc.want)
			}
		})
	}
}
