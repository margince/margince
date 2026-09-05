// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Which hosts count as LinkedIn is decided on both sides of the wire, and the
// two answers are deliberately different sizes.
//
// The SERVER decides what may be STORED as a contact's profile — the
// person_social slot, the vCard import's slot, the classifier that splits a
// card's URL into LinkedIn or website. The CLIENT decides what may be DRAWN
// under the word "LinkedIn", and a label is a claim about where a link goes: a
// stored `https://attacker.example/login` rendered as "LinkedIn" inside the
// product's own chrome is a better phishing surface than an email, because the
// reader already trusts the frame.
//
// So the client's set is the server's plus the shortener, and that gap is the
// thing this holds. Widening the SERVER without the client puts values in the
// slot the rail then refuses to link — a handle the product stored and will not
// show. Widening the CLIENT without a declaration here makes it a link the
// server would never have written and nobody decided to draw.
//
// Both sides are read: the Go sets from the module that owns the write, the
// TypeScript array from the file that owns the label. Neither is this gate's
// own copy, and it fails in both directions.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/people"
)

const frontendWebURL = "../frontend/src/format/weburl.ts"

// tsHostEntry reads one quoted host out of the array literal, in either quote
// style. A host written in the other style than the gate expected would
// otherwise parse as NOTHING, and an entry the parser cannot see is one this
// gate silently agrees with.
var tsHostEntry = regexp.MustCompile(`["']([a-z0-9.-]+\.[a-z]{2,})["']`)

// tsHostComment strips comments before the hosts are read, so a host merely
// MENTIONED in prose beside the array cannot stand in for the entry that was
// deleted.
var tsHostComment = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

func TestTheFrontendLinkedInHostsMatchTheGoDeclaration(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendWebURL)
	if err != nil {
		t.Fatalf("reading the frontend link gate: %v", err)
	}
	const marker = "LINKEDIN_HOSTS"
	start := indexAfter(string(source), marker+" = [")
	if start < 0 {
		t.Fatalf("%s no longer declares %s as an array literal — this gate is reading a shape that is gone",
			frontendWebURL, marker)
	}
	end := indexAfter(string(source)[start:], "]")
	if end < 0 {
		t.Fatalf("%s's %s literal is unterminated", frontendWebURL, marker)
	}

	literal := tsHostComment.ReplaceAllString(string(source)[start:start+end], " ")
	var inTS []string
	for _, m := range tsHostEntry.FindAllStringSubmatch(literal, -1) {
		inTS = append(inTS, m[1])
	}
	if len(inTS) == 0 {
		t.Fatal("no hosts parsed out of the frontend array — a gate that reads nothing agrees with everything")
	}

	storable := people.LinkedInSlotHosts()
	displayOnly := people.LinkedInDisplayOnlyHosts()
	// Both halves populated. An empty storable set would withhold the slot from
	// every value while this gate still balanced, and an empty display-only set
	// would make the assertion below a plain equality that no longer records
	// that the two sides differ ON PURPOSE.
	if len(storable) == 0 || len(displayOnly) == 0 {
		t.Fatalf("LinkedInSlotHosts()=%v, LinkedInDisplayOnlyHosts()=%v — the declaration has lost a half, "+
			"and with it the reason the two sides are allowed to differ", storable, displayOnly)
	}

	declared := append(append([]string{}, storable...), displayOnly...)
	sort.Strings(declared)
	sort.Strings(inTS)
	if strings.Join(declared, ",") != strings.Join(inTS, ",") {
		t.Errorf("the LinkedIn host sets have drifted.\n"+
			"  %s draws: %v\n"+
			"  people declares: %v (storable %v + display-only %v)\n"+
			"A host the client links and the server does not declare is a link nobody decided to draw; "+
			"a host the server would store and the client does not link is a handle the product keeps "+
			"and refuses to show. Add it to whichever half it belongs in, with the reason.",
			frontendWebURL, inTS, declared, storable, displayOnly)
	}
}
