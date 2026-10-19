// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A floor under what an ask may claim about a cold route.
//
// The shape check beside this asks whether the draft names the colleague and
// the contact. It cannot ask the question that matters. A draft can call the
// two of them close while the record says they last spoke a year ago. The
// colleague reading it falsifies that from memory, and the rep who sent it
// looks careless to somebody they work with.
//
// This is a floor and not a solution, said here rather than only in an issue.
// "You two go back a while" overclaims as plainly as "close friends" and
// shares no words with it. So a phrase list cannot catch the general case.
// What it catches is the claim stated outright, the form a model reaches for.

// warmthClaims are read as an assertion about the relationship wherever they
// appear, so a deterministic match on them does not depend on context.
//
// Each is a phrase rather than a word. The corpus for this case lists bare
// "well" and bare "close", which a grader reads in a sentence and a substring
// match cannot. "I hope you are well" and "close of business" are ordinary
// prose, and refusing them would teach a rep the button is broken.
// introwarmth_test.go records that exclusion and its reason.
var warmthClaims = []string{
	"close friend",
	"good friend",
	"old friend",
	"dear friend",
	"we are close",
	"we're close",
	"know each other well",
	"know her well",
	"know him well",
	"know them well",
	"just spoke",
	"spoke recently",
	"recently spoke",
	"spoke just",
	"catching up",
	"caught up recently",
	"speak regularly",
	"talk regularly",
	"speak often",
	"talk often",
	"go back a long way",
	"go back years",
}

// warmthOverclaim answers the claim a cold route's draft may not make, or "".
//
// Only the cold band. A developing route can carry mild warmth and be right
// about it. Widening the floor there would refuse true statements, which costs
// a rep a usable draft and makes no claim safer.
func warmthOverclaim(band, text string) string {
	if band != string(crmcontracts.Company360RouteStrengthBucketCold) {
		return ""
	}
	folded := strings.ToLower(text)
	for _, claim := range warmthClaims {
		if strings.Contains(folded, claim) {
			return claim
		}
	}
	return ""
}
