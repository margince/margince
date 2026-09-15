// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A website read that stopped is classified TWICE, and both answers reach the
// same reader.
//
// The server decides it to word the activity rail — "I've read the company
// website" for a read that filled a bound the product chose, and something
// less settled for one that was interrupted. The browser decides it again to
// pick the tone of the company research panel and the onboarding coverage card.
// The two lists disagreed on `deadline`: the rail said the site was read while
// the panel warned about the same read, so a rep who looked at both got two
// answers to one question and neither surface was obviously wrong.
//
// So the TypeScript list is a declared mirror rather than a second judgement,
// and this is what makes "mirror" true. It compares both directions: a reason
// counted on one side and not the other fails, whichever side added it.

import (
	"os"
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

const frontendSiteReadStops = "../frontend/src/screens/sitereadkind.ts"

// tsStopReason reads one quoted entry out of the TypeScript array.
var tsStopReason = regexp.MustCompile(`["']([a-z_]+)["']`)

// tsStopComment strips comments from the literal's own text first. Without it a
// line inside the array MENTIONING a reason keeps this gate green after the
// real entry is deleted — the failure direction a census must not have.
var tsStopComment = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

func TestTheFrontendCeilingStopsMatchTheGoOnes(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendSiteReadStops)
	if err != nil {
		t.Fatalf("reading the frontend stop list: %v", err)
	}
	const marker = "CONFIGURED_STOPS"
	start := indexAfter(string(source), marker+" = [")
	if start < 0 {
		t.Fatalf("%s no longer declares %s as an array literal — this gate is reading a shape that is gone", frontendSiteReadStops, marker)
	}
	end := indexAfter(string(source)[start:], "]")
	if end < 0 {
		t.Fatalf("%s's %s literal is unterminated", frontendSiteReadStops, marker)
	}

	literal := tsStopComment.ReplaceAllString(string(source)[start:start+end], " ")
	inTS := []string{}
	for _, m := range tsStopReason.FindAllStringSubmatch(literal, -1) {
		inTS = append(inTS, m[1])
	}
	if len(inTS) == 0 {
		t.Fatal("no reasons parsed out of the frontend list — a gate that reads nothing agrees with everything")
	}
	slices.Sort(inTS)

	inGo := contacts.SiteReadOwnCeilings()
	for _, reason := range inGo {
		if !slices.Contains(inTS, reason) {
			t.Errorf("%q is a ceiling the server counts and the browser does not, so the rail will call the read finished while the panel warns about it", reason)
		}
	}
	for _, reason := range inTS {
		if !slices.Contains(inGo, reason) {
			t.Errorf("%q is a ceiling the browser counts and the server does not, so the panel will call the read clean while the rail says it was cut short", reason)
		}
	}
}
