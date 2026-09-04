// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A held message says WHY, or the reader cannot argue with the verdict.
//
// The browser names a hold from a map keyed on the same token the server writes
// into `audience_reason`. That map held five of the nine reasons the derivation
// can write, and the four it omitted were not exotic: `explicitly_confidential`
// is what a confidentiality verdict stamps, and a business mail narrowed to its
// participants therefore told a reader only "not shared with you". The reason
// was on the wire the whole time and the screen dropped it.
//
// That is the failure this gate exists for, and it is the quiet kind: a missing
// entry renders nothing at all, so the screen looks deliberate. Nobody sees a
// raw token and files a bug; they see a limit with no author and conclude the
// product simply does not explain itself.
//
// Both directions are compared. A reason added to the Go constants and not the
// map fails, and so does a map entry naming a token the server cannot write —
// the second is a label nobody will ever read, which is how a map accumulates
// words that once meant something.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
)

const frontendAudienceReasons = "../frontend/src/screens/audiencemembers.tsx"

// tsReasonEntry reads one `token: "catalog.key"` pair out of the map, in either
// spelling an object literal admits.
//
// Quoted keys are matched because a TS-only entry written `"verdict": …` would
// otherwise parse as nothing, and an entry the parser cannot see is one this
// gate silently agrees with. Under-recognition is the one way a census must not
// break.
var tsReasonEntry = regexp.MustCompile(`["']?([a-z_]+)["']?:\s*["']([a-zA-Z.]+)["']`)

// tsComment strips comments before the entries are read, so a line inside the
// literal MENTIONING a token does not keep this gate green after the real entry
// is deleted.
var tsReasonComment = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

func TestTheBrowserNamesEveryAudienceReason(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendAudienceReasons)
	if err != nil {
		t.Fatalf("reading the frontend reason map: %v", err)
	}
	const marker = "AUDIENCE_REASON_LABEL"
	start := indexAfter(string(source), marker+": Record<string, MessageKey> = {")
	if start < 0 {
		t.Fatalf("%s no longer declares %s as a Record<string, MessageKey>; "+
			"this gate reads that literal and cannot judge what it cannot find",
			frontendAudienceReasons, marker)
	}
	body := string(source)[start:]
	if end := strings.Index(body, "\n};"); end >= 0 {
		body = body[:end]
	}
	named := map[string]string{}
	for _, pair := range tsReasonEntry.FindAllStringSubmatch(tsReasonComment.ReplaceAllString(body, ""), -1) {
		named[pair[1]] = pair[2]
	}
	// The floor. A literal the parser reads as empty would otherwise pass every
	// assertion below by vacuity, and report PASS over a screen naming nothing.
	if len(named) == 0 {
		t.Fatalf("no entries parsed out of %s: this gate is judging nothing", marker)
	}

	for _, reason := range activities.EveryReason() {
		if _, ok := named[reason]; !ok {
			t.Errorf("the derivation writes audience_reason %q and the browser has no label for it: "+
				"a message held for that reason says it is not shared and never why, "+
				"which is a verdict the reader cannot correct. Add it to %s in %s, "+
				"with a catalog entry in all three of en/de/vi",
				reason, marker, frontendAudienceReasons)
		}
	}

	// The other direction: a label for a token the server cannot write is one no
	// reader will ever see, and it makes the map look complete while it is not.
	writable := map[string]bool{}
	for _, reason := range activities.EveryReason() {
		writable[reason] = true
	}
	for reason := range named {
		if !writable[reason] {
			t.Errorf("%s names audience_reason %q, which the derivation never writes: "+
				"either the constant was removed and this label outlived it, or the token is "+
				"misspelled and the real reason renders nothing",
				marker, reason)
		}
	}
}

// TestEveryReasonIsListed holds EveryReason against the constants it claims to
// enumerate.
//
// Without it EveryReason is a hand-kept list and the drift merely moves one
// file over: a tenth constant would be written by the derivation, absent from
// EveryReason, and the gate above would report PASS over a browser that cannot
// name it. Go cannot enumerate a const block, so the source is read.
func TestEveryReasonIsListed(t *testing.T) {
	t.Parallel()
	const vocabulary = "../backend/internal/modules/activities/audiencereasons.go"
	source, err := os.ReadFile(vocabulary)
	if err != nil {
		t.Fatalf("reading the reason vocabulary: %v", err)
	}
	// The exported Reason constants, read from their declarations rather than
	// from a list somebody maintains beside them.
	declared := regexp.MustCompile(`(?m)^\s*(Reason[A-Za-z]+)\s*=\s*"([a-z_]+)"`)
	found := map[string]string{}
	for _, pair := range declared.FindAllStringSubmatch(string(source), -1) {
		found[pair[1]] = pair[2]
	}
	if len(found) == 0 {
		t.Fatalf("no Reason constants parsed out of %s: this gate is judging nothing", vocabulary)
	}
	listed := map[string]bool{}
	for _, reason := range activities.EveryReason() {
		listed[reason] = true
	}
	var missing []string
	for name, value := range found {
		if !listed[value] {
			missing = append(missing, name+" ("+value+")")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("activities.EveryReason omits %s: the browser's reason map is held against "+
			"that function, so a constant missing from it is a hold the screen cannot name",
			strings.Join(missing, ", "))
	}
}
