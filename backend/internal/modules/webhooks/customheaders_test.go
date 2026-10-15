// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func testCipher(t *testing.T) *Cipher {
	t.Helper()
	c, err := NewCipher(make([]byte, WebhookKeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestHeadersAreSealedPerSubscriptionAndOpenToTheirValues(t *testing.T) {
	c, sub := testCipher(t), ids.NewV7()
	stored, err := c.sealHeaders(sub, []CustomHeaderInput{
		{Name: "authorization", Value: new("Bearer test-token-not-real")},
		{Name: "X-Tenant", Value: new("acme")},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := headerNames(stored); strings.Join(got, ",") != "Authorization,X-Tenant" {
		t.Fatalf("names = %v, want canonical names in entry order", got)
	}
	for _, h := range stored {
		if strings.Contains(h.SealedValue, "test-token") || strings.Contains(h.SealedValue, "acme") {
			t.Fatalf("sealed value %q carries the plaintext", h.SealedValue)
		}
	}
	opened, err := c.openHeaders(sub, stored)
	if err != nil {
		t.Fatal(err)
	}
	if opened[0].Value != "Bearer test-token-not-real" || opened[1].Value != "acme" {
		t.Fatalf("opened = %+v", opened)
	}
}

func TestASealedValueDoesNotOpenUnderAnotherSubscriptionOrName(t *testing.T) {
	c, sub := testCipher(t), ids.NewV7()
	stored, err := c.sealHeaders(sub, []CustomHeaderInput{{Name: "X-Key", Value: new("k")}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.openHeaders(ids.NewV7(), stored); err == nil {
		t.Fatal("a value sealed for one subscription opened under another")
	}
	moved := []storedHeader{{Name: "X-Other", SealedValue: stored[0].SealedValue}}
	if _, err := c.openHeaders(sub, moved); err == nil {
		t.Fatal("a value sealed for one header opened under another name")
	}
}

func TestARowWithoutAValueKeepsTheStoredOne(t *testing.T) {
	c, sub := testCipher(t), ids.NewV7()
	first, err := c.sealHeaders(sub, []CustomHeaderInput{{Name: "X-Key", Value: new("old")}, {Name: "X-Gone", Value: new("x")}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	next, err := c.sealHeaders(sub, []CustomHeaderInput{{Name: "x-key"}, {Name: "X-New", Value: new("new")}}, first)
	if err != nil {
		t.Fatal(err)
	}
	if next[0].SealedValue != first[0].SealedValue {
		t.Fatal("the kept header was sealed again instead of carried over")
	}
	opened, err := c.openHeaders(sub, next)
	if err != nil {
		t.Fatal(err)
	}
	if len(opened) != 2 || opened[0].Value != "old" || opened[1].Name != "X-New" {
		t.Fatalf("opened = %+v, want X-Key kept and X-Gone dropped", opened)
	}
}

func TestHeaderInputThatCouldMisdirectADeliveryIsRefused(t *testing.T) {
	c, sub := testCipher(t), ids.NewV7()
	many := make([]CustomHeaderInput, maxCustomHeaders+1)
	for i := range many {
		many[i] = CustomHeaderInput{Name: "X-H" + strings.Repeat("a", i+1), Value: new("v")}
	}
	for _, tc := range []struct {
		name   string
		in     []CustomHeaderInput
		reason string
	}{
		{"signature header", []CustomHeaderInput{{Name: "webhook-signature", Value: new("v")}}, "Margince sets webhook-signature"},
		{"any webhook- prefix", []CustomHeaderInput{{Name: "Webhook-Extra", Value: new("v")}}, "reserved"},
		{"content type", []CustomHeaderInput{{Name: "content-type", Value: new("v")}}, "body format"},
		{"proxy header", []CustomHeaderInput{{Name: "Proxy-Authorization", Value: new("v")}}, "reserved"},
		{"host", []CustomHeaderInput{{Name: "Host", Value: new("v")}}, "reserved"},
		{"newline in a value", []CustomHeaderInput{{Name: "X-A", Value: new("a\r\nX-Injected: 1")}}, "line break or control"},
		{"space in a name", []CustomHeaderInput{{Name: "X A", Value: new("v")}}, "letters, digits"},
		{"duplicate ignoring case", []CustomHeaderInput{{Name: "X-A", Value: new("1")}, {Name: "x-a", Value: new("2")}}, "more than once"},
		{"new name without a value", []CustomHeaderInput{{Name: "X-A"}}, "needs a value"},
		{"empty value", []CustomHeaderInput{{Name: "X-A", Value: new("")}}, "needs a value"},
		{"too long a value", []CustomHeaderInput{{Name: "X-A", Value: new(strings.Repeat("v", maxHeaderValueBytes+1))}}, "4 KiB"},
		{"too many", many, "at most 10"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.sealHeaders(sub, tc.in, nil)
			bad, ok := errors.AsType[*BadInputError](err)
			if !ok || bad.Field != fieldCustomHeaders || !strings.Contains(bad.Reason, tc.reason) {
				t.Fatalf("err = %v, want a %s error containing %q", err, fieldCustomHeaders, tc.reason)
			}
		})
	}
}
