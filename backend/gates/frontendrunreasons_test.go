// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The browser puts a troubled run's reason code into words. A server code with
// no words in the browser reads as the generic "something went wrong". That
// hides a reason the server knew. A code only the browser knows
// is a sentence no run can reach. Both directions fail here.

import (
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendRunReasons = "../frontend/src/lib/automationrun.ts"

// tsRunReasonEntry reads one `code: "auto.runs.reasonCode.code"` pair, bare or
// quoted on the left, so an entry in either spelling is seen.
var tsRunReasonEntry = regexp.MustCompile(`["']?([a-z_]+)["']?:\s*"auto\.runs\.reasonCode\.([a-z_]+)"`)

func TestTheFrontendRunReasonWordsMatchTheServerCodes(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendRunReasons)
	if err != nil {
		t.Fatalf("reading the frontend run reason table: %v", err)
	}
	const marker = "RUN_REASON_WORDS: Readonly<Record<string, MessageKey>> = {"
	start := indexAfter(string(source), marker)
	if start < 0 {
		t.Fatalf("%s no longer declares RUN_REASON_WORDS as an object literal", frontendRunReasons)
	}
	end := indexAfter(string(source)[start:], "};")
	if end < 0 {
		t.Fatalf("%s's RUN_REASON_WORDS literal is unterminated", frontendRunReasons)
	}
	table := tsComment.ReplaceAllString(string(source)[start:start+end], " ")
	inTS := map[string]bool{}
	for _, m := range tsRunReasonEntry.FindAllStringSubmatch(table, -1) {
		if m[1] != m[2] {
			t.Errorf("%s: code %q is put into the words of %q", frontendRunReasons, m[1], m[2])
		}
		inTS[m[1]] = true
	}
	if len(inTS) == 0 {
		t.Fatal("no entries parsed out of the frontend table, and a gate that reads nothing agrees with everything")
	}
	inGo := map[string]bool{}
	for _, code := range automation.RunReasonCodes() {
		inGo[code] = true
		if !inTS[code] {
			t.Errorf("the server sends run reason %q and the browser has no words for it", code)
		}
	}
	for code := range inTS {
		if !inGo[code] {
			t.Errorf("the browser has words for run reason %q, which the server never sends", code)
		}
	}
}
