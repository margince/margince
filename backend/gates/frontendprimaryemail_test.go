// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser and the server pick the SAME address to write to, or the address
// a composer prefills and the address a draft is written to differ on a record
// where the reader can see both.
//
// A person carries a LIST of addresses — several, each with a type, a position
// and a primary flag, some retired — so "the contact's email" is a decision
// rather than a field. `persondraft.primaryEmail` makes it for the drafter and
// `frontend/src/format/primaryemail.ts` makes it for every screen, and what
// makes them a mirror is that both are held to ONE table of cases.
//
// The rule they implement has one answer that is actively wrong rather than
// merely different: an ARCHIVED address. Somebody retired it, so mail sent
// there either bounces or reaches a person who asked us to stop. The frontend
// had no shared rule at all before this gate — `people.tsx` picked
// `find(is_primary) ?? [0]`, which offers a retired address whenever it sits
// first — so this is the case the two sides must agree on most.
//
// Compared in BOTH directions, because either alone passes over the drift it is
// meant to catch: a Go case with no TypeScript twin means the browser was never
// held to a rule the server follows, and the reverse for the other side.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const (
	goPrimaryEmailTable = "internal/compose/persondraft/primaryemail_test.go"
	tsPrimaryEmailTable = "../frontend/src/format/primaryemail.test.ts"
)

// The NAME of each case, which is what both tables spell identically. The
// bodies cannot be compared the way the splitter's are — one side builds a
// Person and the other an array of PersonEmail — so the case name is the shared
// text, and each side's own test asserts the answer.
var (
	goCaseName = regexp.MustCompile(`(?m)^\s*name:\s*"([^"\\]*)"`)
	tsCaseName = regexp.MustCompile(`(?m)^\s*it\("([^"\\]*)"`)
)

func TestBothSidesPickTheSameAddress(t *testing.T) {
	t.Parallel()
	goCases := caseNamesIn(t, goPrimaryEmailTable, goCaseName)
	tsCases := caseNamesIn(t, tsPrimaryEmailTable, tsCaseName)

	// A census that can fail short has already failed: a table this cannot
	// parse reads as an empty set, and an empty set agrees with everything.
	if len(goCases) == 0 {
		t.Fatal("no case parsed out of the Go table — either it moved or this gate's " +
			"pattern no longer reads it, and an unread table is one this gate agrees with")
	}
	if len(tsCases) == 0 {
		t.Fatal("no case parsed out of the TypeScript table — same failure, other side")
	}

	for name := range goCases {
		if !tsCases[name] {
			t.Errorf("the server's address picker is held to a case the browser is not:\n  %q\n"+
				"add it to %s, or the two are free to disagree about it", name, tsPrimaryEmailTable)
		}
	}
	for name := range tsCases {
		if !goCases[name] {
			t.Errorf("the browser's address picker is held to a case the server is not:\n  %q\n"+
				"add it to %s, or the two are free to disagree about it", name, goPrimaryEmailTable)
		}
	}
}

// caseNamesIn reads the case names out of one table.
//
// Comments are stripped first: a name quoted inside prose would otherwise count
// as a case that exists, and this gate would report parity the tables do not
// have. It reuses commentPattern from the splitter's gate beside it — one
// spelling of "what a comment looks like" for both.
func caseNamesIn(t *testing.T, path string, pattern *regexp.Regexp) map[string]bool {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	text := commentPattern.ReplaceAllString(string(source), "")
	out := map[string]bool{}
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		if name := strings.TrimSpace(match[1]); name != "" {
			out[name] = true
		}
	}
	return out
}
