// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The search box picks a Vietnamese data question by its own word list. The
// server holds a won count to the Sales won metric by another list. A count
// phrase only the browser knows lets a record list answer a won count.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/searchplan"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendQuestionWords = "../frontend/src/lib/question.ts"

// The Vietnamese data phrases that ask no count and say no win, so the
// won-count admission has nothing to hold them to.
var vietnameseDataWithoutWonCount = gatekit.Waive(map[string]string{
	"tổng":          "a sum; the planner picks its metric",
	"doanh thu":     "revenue, a money figure",
	"đạt mục tiêu":  "reaching a target, read against the goal",
	"đạt chỉ tiêu":  "reaching a quota, read against the goal",
	"vượt mục tiêu": "beating a target, read against the goal",
})

var tsPhrase = regexp.MustCompile(`"([^"]+)"`)

// tsArray returns the quoted strings of the array literal that opens after
// marker, and whether it spreads VIETNAMESE_COUNTS.
func tsArray(t *testing.T, source, marker string) ([]string, bool) {
	t.Helper()
	start := indexAfter(source, marker)
	if start < 0 {
		t.Fatalf("%s no longer holds %q; this gate reads a shape that is gone", frontendQuestionWords, marker)
	}
	end := strings.Index(source[start:], "]")
	if end < 0 {
		t.Fatalf("%s: the array after %q is unterminated", frontendQuestionWords, marker)
	}
	body := gatekit.TSWithoutComments(source[start : start+end])
	var phrases []string
	for _, m := range tsPhrase.FindAllStringSubmatch(body, -1) {
		phrases = append(phrases, m[1])
	}
	return phrases, strings.Contains(body, "...VIETNAMESE_COUNTS")
}

func TestEveryVietnameseDataPhraseHasAServerCounterpart(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(frontendQuestionWords)
	if err != nil {
		t.Fatalf("reading the search box word table: %v", err)
	}
	source := string(raw)
	counts, _ := tsArray(t, source, "const VIETNAMESE_COUNTS = [")
	vi := indexAfter(source, "\n  vi: {")
	if vi < 0 {
		t.Fatalf("%s declares no vi entry", frontendQuestionWords)
	}
	data, spreads := tsArray(t, source[vi:], "data: [")
	if spreads {
		data = append(data, counts...)
	}
	if len(counts) == 0 || len(data) == 0 {
		t.Fatal("no Vietnamese phrases parsed out of the search box table; a gate that reads nothing agrees with everything")
	}

	for _, phrase := range data {
		if !searchplan.AsksVietnameseWonCount(phrase) && !vietnameseDataWithoutWonCount.Waived(t, phrase) {
			t.Errorf("%q sends a question to the data reader, and the won-count admission in searchplan/vietnamese.go does not read it. Add it there, or declare why it asks no count in vietnameseDataWithoutWonCount", phrase)
		}
	}
	vietnameseDataWithoutWonCount.AssertAllMatched(t)
}
