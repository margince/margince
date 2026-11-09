// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package originalstore

import (
	"fmt"
	"strings"
	"testing"
)

func TestSubjectHintsKeepEveryLiteralEmailMention(t *testing.T) {
	for _, email := range []string{"a@b", "sam@example.test", "long.name+tag@other.example.test", "A.CASE@EXAMPLE.TEST"} {
		for _, body := range []string{email, "prefix" + email + "suffix", `{"email":"` + email + `"}`, strings.ToUpper(email), strings.Repeat("x", 10000) + email} {
			hints := subjectHints(strings.ToLower(body), nil)
			if !hints.MayMentionEmail(email) {
				t.Fatalf("filter ruled out a literal mention of %q", email)
			}
			if len(hints.EmailBits) > SubjectHintLimit {
				t.Fatal("filter grew with the original")
			}
		}
	}
	hints := subjectHints("alice@first.example.test", nil)
	if hints.MayMentionEmail("bob@second.example.test") {
		t.Fatal("filter never rules out unrelated evidence")
	}
	for _, legacy := range []*SubjectHints{nil, {Version: 2, EmailBits: make([]uint16, 0)}, {Version: 1}} {
		if !legacy.MayMentionEmail("alice@example.test") {
			t.Fatal("unknown hint format excluded evidence")
		}
	}
	if !hints.MayMentionEmail("søren@example.test") {
		t.Fatal("unsupported Unicode search was excluded")
	}
}

func TestSaturatedSubjectHintsKeepLaterMentions(t *testing.T) {
	var addresses []string
	for i := range 2000 {
		addresses = append(addresses, fmt.Sprintf("sender-%04d@domain-%04d.example", i, i))
	}
	hints := subjectHints(strings.Join(addresses, " "), nil)
	if !hints.EmailSaturated || len(hints.EmailBits) > SubjectHintLimit {
		t.Fatal("dense evidence did not fall back within the hint bound")
	}
	for _, email := range []string{addresses[0], addresses[len(addresses)-1], "later@example.test"} {
		if !hints.MayMentionEmail(email) {
			t.Fatal("a saturated filter excluded evidence")
		}
	}
}
