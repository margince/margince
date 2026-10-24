// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// last_activity_at on deal, contact and company skips a task and a called-off
// meeting, and counts only what has happened. relstrength.LastActivityCountsSQL
// spells all three.
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
// move. last_activity_of_project keeps its own meeting rule and is not one of
// them, but drops tasks as they do.
var meetingRuleClocks = []string{"last_activity_of_deal", "last_activity_of_contact", "last_activity_of_company"}

// projectClock is the recency function that carries only the kind bound.
const projectClock = "last_activity_of_project"

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
		if missing := armsMissing(body, rule); len(missing) > 0 {
			t.Errorf("%s: activity arms %v of %d do not carry %q, so a task, a canceled or no-show meeting, "+
				"or one still ahead, can keep the record looking recently active. Replace the function in a "+
				"new migration with the rule's current text in every arm.", name, missing, len(arms), rule)
		}
	}
	if body, ok := bodies[projectClock]; !ok {
		t.Errorf("no migration defines %s, so this gate checks nothing for it", projectClock)
	} else if missing := armsMissing(body, relstrength.LastActivityKindSQL("a")); len(missing) > 0 {
		t.Errorf("%s: activity arms %v do not carry %q, so filing a task moves the project's last activity",
			projectClock, missing, relstrength.LastActivityKindSQL("a"))
	}
	if src, err := os.ReadFile(projectRecencyReader); err != nil {
		t.Errorf("%s: %v", projectRecencyReader, err)
	} else if !strings.Contains(string(src), "relstrength.LastActivityKindSQL(") {
		t.Errorf("%s no longer calls relstrength.LastActivityKindSQL, so the project page's last activity "+
			"counts a task that last_activity_of_project does not", projectRecencyReader)
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

// projectRecencyReader reads a project's last activity for the project page
// beside the stored column.
const projectRecencyReader = "internal/modules/activities/projectcoverage.go"

// armsMissing answers the 1-based activity arms of body that do not carry rule.
func armsMissing(body, rule string) []int {
	var missing []int
	for i, arm := range activityArms(body) {
		if !strings.Contains(arm, rule) {
			missing = append(missing, i+1)
		}
	}
	return missing
}

// TestTheClockGateSeesAMissingKindBound proves the arm check fails on a body
// that keeps the meeting and time halves of the rule and drops the kind half.
func TestTheClockGateSeesAMissingKindBound(t *testing.T) {
	t.Parallel()
	rule := relstrength.LastActivityCountsSQL("a", "now()")
	kind := relstrength.LastActivityKindSQL("a")
	if !strings.Contains(rule, kind) {
		t.Fatalf("LastActivityCountsSQL %q does not carry the kind bound %q", rule, kind)
	}
	stripped := strings.Replace(rule, kind+" AND ", "", 1)
	body := "SELECT max(a.occurred_at) FROM activity_link l JOIN activity a ON a.id = l.activity_id AND " + stripped
	if missing := armsMissing(body, rule); len(missing) != 1 {
		t.Fatalf("a body without the kind bound passed the arm check: %q", body)
	}
	if missing := armsMissing(body, kind); len(missing) != 1 {
		t.Fatalf("a project body without the kind bound passed the arm check: %q", body)
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
