// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"strings"
	"testing"
)

func TestABuilderSecretMustBeLongEnoughNotToBeGuessed(t *testing.T) {
	for secret, ok := range map[string]bool{
		"short":                  false,
		strings.Repeat("s", 16):  true,
		strings.Repeat("s", 512): true,
		strings.Repeat("s", 513): false,
		"":                       true,
	} {
		if err := checkSenderSecret(&secret); (err == nil) != ok {
			t.Errorf("a %d-character secret: err = %v, want accepted %v", len(secret), err, ok)
		}
	}
	if err := checkSenderSecret(nil); err != nil {
		t.Errorf("no secret: %v", err)
	}
}

func TestAWebsiteListTakesOnlySecureOrigins(t *testing.T) {
	got, err := normalizeOrigins([]string{"https://WWW.Example.com/contact?x=1", "http://localhost:3000"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != "https://www.example.com http://localhost:3000" {
		t.Fatalf("normalizeOrigins = %v", got)
	}
	for _, refused := range []string{"http://example.com", "https://user@example.com", "null", "example.com"} {
		if _, err := normalizeOrigins([]string{refused}); err == nil {
			t.Errorf("%q was accepted as a website", refused)
		}
	}
}
