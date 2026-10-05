// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

// EnsureGreeting puts the floor's greeting line above a body that still opens
// without one once the correction retry is spent, so no draft is served
// ungreeted. A body that greets is returned unchanged.
func EnsureGreeting(body string, envelope draftfloor.Envelope, firstName, lastName string) string {
	if !missingGreeting(body, firstName, lastName) {
		return body
	}
	return envelope.Greeting(firstName, lastName) + "\n\n" + strings.TrimSpace(body)
}

// greetingOpeners are the words a greeting starts with, in every language a
// draft is written in. One list rather than one per language: a draft's first
// line is judged on its own words, and a draft in the wrong language is a
// different defect with its own rule.
var greetingOpeners = []string{
	"hi", "hello", "hey", "dear", "good morning", "good afternoon", "good evening",
	"hallo", "guten tag", "guten morgen", "guten abend", "sehr geehrte",
	"sehr geehrter", "liebe", "lieber", "moin", "servus", "grüß gott", "grüezi",
	"xin chào", "chào", "kính gửi", "thân gửi", "gửi",
}

// missingGreeting reports a body whose first line does not greet anybody.
//
// A first line greets when it opens on a greeting word, or on the recipient's
// own name followed by a comma or an exclamation mark ("Greven,"). A run-on
// greeting still counts: RuleUnbrokenBlock is the rule for a greeting with no
// line of its own.
func missingGreeting(body, firstName, lastName string) bool {
	first, _, _ := strings.Cut(strings.TrimSpace(body), "\n")
	first = strings.ToLower(strings.TrimSpace(first))
	if first == "" {
		return false
	}
	for _, opener := range greetingOpeners {
		if wordIndex(first, opener) == 0 {
			return false
		}
	}
	return !opensOnName(first, firstName, lastName)
}

// opensOnName reports a line that starts with one of the recipient's names,
// whole, and then a comma or an exclamation mark.
func opensOnName(line, firstName, lastName string) bool {
	full := strings.TrimSpace(firstName + " " + lastName)
	for _, name := range []string{full, firstName, lastName} {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" || !strings.HasPrefix(line, name) {
			continue
		}
		if rest := line[len(name):]; strings.HasPrefix(rest, ",") || strings.HasPrefix(rest, "!") {
			return true
		}
	}
	return false
}
