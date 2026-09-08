// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testmailbox

import (
	"context"
	"errors"
	"go/build"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestSendEmailRefusesAddressesOutsideTheQuarantine(t *testing.T) {
	cases := []struct {
		name string
		msg  connector.EmailMessage
	}{
		{"to outside reserved domain", connector.EmailMessage{MessageID: "a@test.example", To: []string{"real.person@gmail.com"}}},
		{"cc outside reserved domain", connector.EmailMessage{MessageID: "b@test.example", To: []string{"buyer@example.com"}, Cc: []string{"real.person@gmail.com"}}},
		{"bcc outside reserved domain", connector.EmailMessage{MessageID: "c@test.example", To: []string{"buyer@example.com"}, Bcc: []string{"real.person@gmail.com"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := New(&fakeLedger{})
			_, err := c.SendEmail(context.Background(), testAuth(t, ids.NewV7().String()), tc.msg)
			if !errors.Is(err, connector.ErrRecipientUnreachable) {
				t.Errorf("SendEmail(%+v) error = %v, want ErrRecipientUnreachable", tc.msg, err)
			}
		})
	}
}

func TestSendEmailAcceptsEveryReservedDomain(t *testing.T) {
	reserved := []string{"buyer@example.com", "buyer@example.net", "buyer@example.org", "buyer@thing.test", "buyer@thing.example", "buyer@thing.invalid", "buyer@thing.localhost"}
	for _, addr := range reserved {
		t.Run(addr, func(t *testing.T) {
			c := New(&fakeLedger{})
			_, err := c.SendEmail(context.Background(), testAuth(t, ids.NewV7().String()), connector.EmailMessage{MessageID: "x@test.example", To: []string{addr}})
			if err != nil {
				t.Errorf("SendEmail to reserved address %q = %v, want nil", addr, err)
			}
		})
	}
}

func TestSendReceiptIsIdempotentOnMessageID(t *testing.T) {
	c := New(&fakeLedger{})
	auth := testAuth(t, ids.NewV7().String())
	msg := connector.EmailMessage{MessageID: "same@test.example", To: []string{"buyer@example.com"}}
	r1, err := c.SendEmail(context.Background(), auth, msg)
	if err != nil {
		t.Fatalf("first SendEmail: %v", err)
	}
	r2, err := c.SendEmail(context.Background(), auth, msg)
	if err != nil {
		t.Fatalf("second SendEmail: %v", err)
	}
	if r1.ProviderMessageID != r2.ProviderMessageID {
		t.Errorf("ProviderMessageID differs across calls with the same MessageID: %q vs %q — violates the SendEmail idempotency contract", r1.ProviderMessageID, r2.ProviderMessageID)
	}
	if r1.RFC822MessageID != "" {
		t.Errorf("RFC822MessageID = %q, want empty (correct no-op: this provider honours the identity it was given)", r1.RFC822MessageID)
	}
}

func TestSendEmailValidatesTheMessageFirst(t *testing.T) {
	c := New(&fakeLedger{})
	_, err := c.SendEmail(context.Background(), testAuth(t, ids.NewV7().String()), connector.EmailMessage{MessageID: "not valid", To: []string{"buyer@example.com"}})
	if !errors.Is(err, connector.ErrInvalidMessageID) {
		t.Errorf("SendEmail with an invalid MessageID = %v, want ErrInvalidMessageID", err)
	}
}

func TestSendEmailRecordsTheSendForItsOwnEcho(t *testing.T) {
	ledger := &fakeLedger{}
	c := New(ledger)
	userID := ids.NewV7()
	if _, err := c.SendEmail(context.Background(), testAuth(t, userID.String()), connector.EmailMessage{
		MessageID: "recorded@test.example", To: []string{"buyer@example.com"}, Subject: "Hi",
	}); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if len(ledger.recorded) != 1 {
		t.Fatalf("ledger.recorded = %+v, want one entry", ledger.recorded)
	}
	got := ledger.recorded[0]
	if got.userID != userID.String() || got.messageID != "recorded@test.example" || got.subject != "Hi" {
		t.Errorf("recorded = %+v, want userID=%s messageID=recorded@test.example subject=Hi", got, userID)
	}
}

// TestPackageNeverImportsTheNetwork is the honest fitness test for "never
// reaches the network" when there is no HTTP client to inject a spy for:
// this package's own source must not import net or net/http, structurally.
func TestPackageNeverImportsTheNetwork(t *testing.T) {
	fset := token.NewFileSet()
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("build.ImportDir: %v", err)
	}
	for _, name := range append(append([]string{}, pkg.GoFiles...), pkg.TestGoFiles...) {
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if path == "net" || strings.HasPrefix(path, "net/") {
				t.Errorf("%s imports %q — test_mailbox must never reach the network", name, path)
			}
		}
	}
}
