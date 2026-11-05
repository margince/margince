// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"strings"
	"testing"
)

// The suppression hash is a matching contract between the eraser and
// every ingest path: normalization must be identical on both sides,
// and a pattern-metacharacter identifier must match itself, never act
// as a wildcard.

func TestSuppressionHashNormalizes(t *testing.T) {
	base := SuppressionHash("selma@example.test")
	for _, variant := range []string{"SELMA@example.test", "  selma@example.test  ", "Selma@Example.Test"} {
		if SuppressionHash(variant) != base {
			t.Errorf("variant %q hashes differently — a trivial respelling would resurrect the subject", variant)
		}
	}
	if SuppressionHash("other@example.test") == base {
		t.Error("distinct identifiers collide")
	}
}

// The channel key must separate the two fields it joins, and must separate
// providers: two accounts whose ids differ only in where the boundary falls
// are different humans, and the same numeric id on two providers is two
// contacts. A key that collided would suppress a stranger.
func TestChannelIdentityHashSeparatesProviderFromAccount(t *testing.T) {
	base := ChannelIdentityHash("telegram", "123456789")
	others := map[string]string{
		"telegram":   "12345678",  // a neighbouring account on the same provider
		"zalo":       "123456789", // the same digits on another provider
		"telegram:1": "23456789",  // the same characters, boundary shifted
	}
	for provider, channelUserID := range others {
		if ChannelIdentityHash(provider, channelUserID) == base {
			t.Errorf("%q/%q collides with telegram/123456789 — an erasure would suppress a different account",
				provider, channelUserID)
		}
	}
	if ChannelIdentityHash("Telegram", " 123456789 ") != base {
		t.Error("the channel key skips the normalization every suppression key shares")
	}
}

func TestEscapeLikeNeutralizesWildcards(t *testing.T) {
	cases := map[string]string{
		`a%b@example.test`: `a\%b@example.test`,
		`a_b@example.test`: `a\_b@example.test`,
		`a\b@example.test`: `a\\b@example.test`,
		`plain@example.te`: `plain@example.te`,
	}
	for in, want := range cases {
		if got := EscapeLike(in); got != want {
			t.Errorf("EscapeLike(%q) = %q, want %q", in, got, want)
		}
	}
}

// The SQL chain must escape in the same order as EscapeLike (backslash first).
func TestEscapeLikeSQLEscapesBackslashFirst(t *testing.T) {
	const want = `replace(replace(replace(x, '\', '\\'), '%', '\%'), '_', '\_')`
	if got := EscapeLikeSQL("x"); got != want {
		t.Errorf("EscapeLikeSQL(x) = %s, want %s", got, want)
	}
}

// Every quick-find, with or without the identifier arm, escapes the typed text.
func TestQuickFindEscapesTheTypedText(t *testing.T) {
	const folded = "f_fold_apostrophes(lower($3))"
	for name, clause := range map[string]string{
		"plain":      QuickFindClause(3, "name"),
		"identifier": QuickFindClauseWith(3, "name", Identifier{Table: "contact_email", FK: "contact_id", Column: "email"}),
	} {
		if !strings.Contains(clause, EscapeLikeSQL(folded)) {
			t.Errorf("%s quick-find does not escape the typed text: %s", name, clause)
		}
	}
}

func TestAnIdentifierHasAnExactArmAndAnEscapedContainsArm(t *testing.T) {
	id := ContactEmails
	exact := id.ExactSQL(3, "id")
	if strings.Contains(exact, "LIKE") {
		t.Errorf("the exact arm must be an equality: %s", exact)
	}
	contains := id.ContainsSQL(3, "id")
	if !strings.Contains(contains, EscapeLikeSQL("f_fold_apostrophes(lower(btrim($3)))")) {
		t.Errorf("the contains arm does not escape the typed text: %s", contains)
	}
	if !strings.Contains(contains, "archived_at IS NULL") {
		t.Errorf("the contains arm reads archived rows: %s", contains)
	}
	match := id.MatchSQL(3, "id")
	if !strings.Contains(match, exact) || !strings.Contains(match, contains) {
		t.Errorf("MatchSQL must be both arms: %s", match)
	}
}

func TestDomainTermTakesThePartAfterTheAt(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"a@example.com": "example.com", "weber@exam": "exam", "example": "example", "a@": "a@"} {
		if got := DomainTerm(in); got != want {
			t.Errorf("DomainTerm(%q) = %q, want %q", in, got, want)
		}
	}
}
