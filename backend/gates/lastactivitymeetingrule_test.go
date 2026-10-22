// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// last_activity_at on deal, contact and company skips a called-off meeting and
// counts only what has happened. relstrength.LastActivityCountsSQL spells both.
//
// The last_activity_of_* SQL functions fold the column and cannot call Go, so
// their migration carries the rule's rendered text. This gate holds every arm
// to it, and holds the Go reads of the same question to the same call.

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
	"internal/modules/deals/health.go":         "deal health's recency evidence",
	"internal/modules/contacts/lead_read.go":   "a lead's last activity",
	"internal/compose/contact360/lasttouch.go": "a contact's last inbound and outbound touch",
	"internal/modules/deals/quietfacts.go":     "a deal's last interaction each way, cited in the close-date review",
	"internal/modules/activities/lasttouch.go": "the last genuine engagement that holds back a no-activity reminder",
	"internal/modules/deals/lastemail.go":      "the newest email on a deal's board card",
}

func TestNoLastActivityClockCountsACalledOffMeeting(t *testing.T) {
	t.Parallel()
	bodies := currentRecencyBodies(t)
	rule := relstrength.LastActivityCountsSQL("a", "now()")
	for _, name := range meetingRuleClocks {
		body, ok := bodies[name]
		if !ok {
			t.Errorf("no migration defines %s, so this gate checks nothing for it", name)
			continue
		}
		arms := activityArms(body)
		if len(arms) == 0 {
			t.Errorf("%s: no activity join found, so the gate cannot see this function's shape", name)
			continue
		}
		for i, arm := range arms {
			if !strings.Contains(arm, rule) {
				t.Errorf("%s: activity arm %d of %d does not carry %q, so a canceled or no-show meeting, "+
					"or one still ahead, can keep the record looking recently active. Replace the function in a "+
					"new migration with the rule's current text in every arm.", name, i+1, len(arms), rule)
			}
		}
	}
	for file, what := range meetingRuleReaders {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("%s (%s): %v", file, what, err)
			continue
		}
		if !strings.Contains(string(src), "relstrength.LastActivityCountsSQL(") {
			t.Errorf("%s no longer calls relstrength.LastActivityCountsSQL, so %s can count a called-off "+
				"meeting, or one still ahead, as a touch", file, what)
		}
	}
}

// activityArms splits a function body at UNION ALL and answers the parts that
// join activity; a function without a union is one arm. Each arm is checked on
// its own, so a rule written twice in one cannot cover its absence in another.
func activityArms(body string) []string {
	var arms []string
	for part := range strings.SplitSeq(body, "UNION ALL") {
		if strings.Contains(part, "activity a ON a.id = l.activity_id") {
			arms = append(arms, part)
		}
	}
	return arms
}
