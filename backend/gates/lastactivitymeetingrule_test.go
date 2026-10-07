// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// last_activity_at on deal, contact and company skips a called-off meeting by
// the same rule as contact strength, spelled once in relstrength.
//
// The column is folded by the last_activity_of_* SQL functions, which cannot
// call Go, so their migration carries the rule's rendered text. This gate holds
// every arm of each to relstrength.NotCalledOffSQL, and holds the Go reads that
// ask the same question without the stored column to the same call.

import (
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// meetingRuleClocks are the recency functions a called-off meeting must not
// move. last_activity_of_project keeps its own rule and is not one of them.
var meetingRuleClocks = []string{"last_activity_of_deal", "last_activity_of_contact", "last_activity_of_company"}

// gatekit:fixture the Go reads of a record's last touch that do not read
// last_activity_at, each with what it computes.
var meetingRuleReaders = map[string]string{
	"internal/modules/deals/health.go":       "deal health's recency evidence",
	"internal/modules/contacts/lead_read.go": "a lead's last activity",
}

func TestNoLastActivityClockCountsACalledOffMeeting(t *testing.T) {
	t.Parallel()
	bodies := currentRecencyBodies(t)
	rule := relstrength.NotCalledOffSQL("a")
	for _, name := range meetingRuleClocks {
		body, ok := bodies[name]
		if !ok {
			t.Errorf("no migration defines %s, so this gate checks nothing for it", name)
			continue
		}
		arms := strings.Count(body, "activity a ON a.id = l.activity_id")
		if arms == 0 {
			t.Errorf("%s: no activity join found, so the gate cannot see this function's shape", name)
			continue
		}
		if got := strings.Count(body, rule); got < arms {
			t.Errorf("%s: %d activity arm(s) but only %d carry %q, so a canceled or no-show meeting "+
				"can keep the record looking recently active. Replace the function in a new migration "+
				"with the rule's current text in every arm.", name, arms, got, rule)
		}
	}
	for file, what := range meetingRuleReaders {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("%s (%s): %v", file, what, err)
			continue
		}
		if !strings.Contains(string(src), "relstrength.NotCalledOffSQL(") {
			t.Errorf("%s no longer calls relstrength.NotCalledOffSQL, so %s can count a called-off "+
				"meeting as a touch", file, what)
		}
	}
}
