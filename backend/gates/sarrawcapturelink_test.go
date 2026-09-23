// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// The SAR's raw-capture correlation is spelled ONCE.
//
// The clause asks it four times — twice to decide whether a provider original's
// content may be disclosed, twice to withhold the payload on the same answer.
// Four copies of a disclosure predicate is four chances for one of them to
// drift, on the one surface where drifting IS the disclosure: a copy that
// stayed on the old (source_system, source_id) join would go on withholding
// every Telegram original, and a copy that lost the sibling arm would hand over
// a held message's text along with the open one beside it.
//
// It reads the SOURCE rather than the behaviour because that is what the claim
// is about. A second spelling that happens to agree today is still a second
// thing to keep in step, and the failure it leads to is not visible in any
// answer — a subject access request returns fewer rows, or more, and nobody who
// reads it knows which was intended.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// sarSectionsFile is relative to the module root, which is where the gates
// package runs — the same spelling weeklymailorder_test.go used.
const sarSectionsFile = "internal/modules/privacy/sarsections.go"

// rawCaptureCorrelation matches any join between an activity and a raw_capture
// row, in either of the two spellings the fix had to choose between.
var rawCaptureCorrelation = regexp.MustCompile(
	`a\.raw_capture_id\s*=\s*rc\.id|a\.source_id\s*=\s*rc\.source_id`)

func TestTheSARRawCaptureCorrelationIsSpelledOnce(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(sarSectionsFile)
	if err != nil {
		t.Fatalf("reading %s: %v", sarSectionsFile, err)
	}

	// The constant's own body is the ONE spelling. Cut it out and nothing that
	// correlates the two tables may remain: every other use goes through the
	// name.
	rest, ok := withoutTheLinkConstant(string(src))
	if !ok {
		t.Fatal("sarRawCaptureLink is gone from " + sarSectionsFile + ", so the clause that " +
			"disclosed a provider original now decides it inline — which is one edit away from " +
			"being copied to the next arm that needs it")
	}
	if loose := rawCaptureCorrelation.FindAllString(rest, -1); len(loose) > 0 {
		t.Errorf("%s correlates activity to raw_capture %d time(s) outside sarRawCaptureLink: %v\n\n"+
			"Every use must go through the name. A second copy decides disclosure on a subject "+
			"access request by itself, and the two disagreeing shows up as an export with the "+
			"wrong rows in it rather than as a failure",
			sarSectionsFile, len(loose), loose)
	}
}

// withoutTheLinkConstant answers the file with sarRawCaptureLink's declaration
// removed, and whether it was there at all.
//
// The raw string runs to the backtick that closes it, so the cut is exact
// rather than a line count that a reformatting would slide off.
func withoutTheLinkConstant(src string) (string, bool) {
	const decl = "const sarRawCaptureLink = `"
	open := strings.Index(src, decl)
	if open < 0 {
		return src, false
	}
	end := strings.Index(src[open+len(decl):], "`")
	if end < 0 {
		return src, false
	}
	return src[:open] + src[open+len(decl)+end+1:], true
}
