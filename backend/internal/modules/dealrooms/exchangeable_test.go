// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

import (
	"os"
	"strings"
	"testing"
)

// Peek and exchange are the two statements that decide whether a credential
// admits its bearer. They must apply exchangeablePredicate and nothing beside
// it, so a rule added to one cannot be missing from the other.
func TestPeekAndExchangeApplyTheOneExchangeablePredicate(t *testing.T) {
	source, err := os.ReadFile("store_public.go")
	if err != nil {
		t.Fatalf("reading store_public.go: %v", err)
	}
	text := string(source)
	if applied := strings.Count(text, "`+exchangeablePredicate+"); applied != 2 {
		t.Errorf("exchangeablePredicate is applied in %d statements, want the two (peek and exchange)", applied)
	}
	// A second spelling of the credential rule would match the token column
	// outside the predicate's own declaration.
	declared := strings.Index(text, "var exchangeablePredicate")
	if declared < 0 || strings.Contains(text[:declared], "i.token_hash") ||
		strings.Count(text[declared:], "i.token_hash") != 1 {
		t.Error("the credential rule is spelled somewhere other than exchangeablePredicate")
	}
}
