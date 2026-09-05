// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The order of the opt-out check against the claim.
//
// A unit test over the SOURCE rather than over a running job, because what is
// being asserted is an ordering that no fixture can make visible: both orders
// send nothing to an opted-out rep, and the difference only shows up a week
// later when that rep turns their mail back on and the attempt is already
// spent. A test that ran the job would pass on the broken order.

import (
	"os"
	"strings"
	"testing"
)

// The opt-out is read BEFORE the claim is spent.
//
// ClaimMailAttempt stamps mail_attempted_at permanently and answers false ever
// after, so a check placed below it burns the rep's one attempt for the week.
// If they then switch delivery back on, the week they changed their mind in can
// never be sent — and nothing anywhere says why.
func TestTheOptOutIsCheckedBeforeTheAttemptIsSpent(t *testing.T) {
	source, err := os.ReadFile("weeklymailjobs.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)

	optOut := strings.Index(body, "w.optedOutOfWeekly(ctx)")
	claim := strings.Index(body, "w.engine.ClaimMailAttempt(")
	if optOut < 0 {
		t.Fatal("no opt-out check found in the weekly mail path — a delivery " +
			"preference nothing reads is a control that does nothing")
	}
	if claim < 0 {
		t.Fatal("no claim found in the weekly mail path, so this test judged nothing")
	}
	if optOut > claim {
		t.Error("the opt-out is checked AFTER the claim is spent. " +
			"ClaimMailAttempt stamps the attempt permanently, so an opted-out rep " +
			"burns their one attempt for the week — and if they turn delivery back " +
			"on, that week can never be sent")
	}
}

// The read fails OPEN.
//
// A rep who wanted their weekly and did not get it is worse served than one who
// gets a message they meant to switch off: the second is an annoyance they can
// fix from the same page, the first is silence they have no way to notice.
func TestAnUnreadablePreferenceStillSends(t *testing.T) {
	source, err := os.ReadFile("weeklymailjobs.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)

	start := strings.Index(body, "func (w *weeklyGenerateWorker) optedOutOfWeekly")
	if start < 0 {
		t.Fatal("no preference read found, so this test judged nothing")
	}
	fn := body[start:]
	end := strings.Index(fn, "\n}")
	if end < 0 {
		t.Fatal("the preference read has no closing brace — the walk is broken")
	}
	if !strings.Contains(fn[:end], "return false") {
		t.Error("the preference read does not fail open — an error reading a " +
			"setting must not silence a rep's weekly")
	}
}
