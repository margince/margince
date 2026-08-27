// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"regexp"
	"strconv"
	"testing"
)

// The lock statement binds every placeholder it names, and names no more than
// it binds.
//
// It reuses the destroy statement's own id-set fragments so a row cannot be
// judged by one spelling and locked by another — but those fragments are
// written for the destroy's six-argument list, where the channel keys are $6
// behind the floor and the tombstone name. This statement wants three
// arguments, so one placeholder is renumbered.
//
// That renumbering is the fragile part, and it fails LOUDLY but late: Postgres
// answers `could not determine data type of parameter $3` at run time, which is
// an integration failure in whatever suite happens to erase a person. A
// fragment that grows a $7 tomorrow would do it again. This is the cheap check
// that catches it in a unit test instead.
func TestTheTimelineLockBindsEveryPlaceholderItNames(t *testing.T) {
	t.Parallel()

	const bound = 3 // personID, emails, channelKeys
	seen := map[int]bool{}
	for _, match := range regexp.MustCompile(`\$(\d+)`).FindAllStringSubmatch(subjectTimelineLockSQL, -1) {
		n, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatalf("unreadable placeholder %q", match[0])
		}
		seen[n] = true
		if n > bound {
			t.Errorf("the lock names $%d and binds %d arguments — Postgres refuses a placeholder "+
				"nothing types, as `could not determine data type of parameter`, at run time in "+
				"whichever suite erases a person next", n, bound)
		}
	}
	// And every one it binds is used: an argument the statement never mentions
	// is the same refusal from the other side.
	for n := 1; n <= bound; n++ {
		if !seen[n] {
			t.Errorf("the lock binds $%d and never names it", n)
		}
	}
}
