// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// deal.last_activity_at skips a called-off meeting by the same rule as contact
// strength, spelled once in relstrength.
//
// The column is folded by last_activity_of_deal, a SQL function that cannot
// call Go, so its migration carries the rule's rendered text. This gate holds
// that copy to relstrength.NotCalledOffSQL, and holds deal health's evidence
// read, which must name the row the column counts, to the same call.

import (
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

const dealHealthReader = "internal/modules/deals/health.go"

func TestTheDealClockSkipsACalledOffMeeting(t *testing.T) {
	t.Parallel()
	body, ok := currentRecencyBodies(t)["last_activity_of_deal"]
	if !ok {
		t.Fatal("no migration defines last_activity_of_deal, so this gate checks nothing")
	}
	if rule := relstrength.NotCalledOffSQL("a"); !strings.Contains(body, rule) {
		t.Errorf("last_activity_of_deal's current body does not carry %q, so a canceled or no-show "+
			"meeting can keep a deal looking recently active. Replace the function in a new "+
			"migration with the rule's current text.", rule)
	}
	src, err := os.ReadFile(dealHealthReader)
	if err != nil {
		t.Fatalf("%s: %v", dealHealthReader, err)
	}
	if !strings.Contains(string(src), "relstrength.NotCalledOffSQL(") {
		t.Errorf("%s no longer calls relstrength.NotCalledOffSQL, so its recency evidence can cite "+
			"a called-off meeting that last_activity_of_deal does not count", dealHealthReader)
	}
}
