// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A deal's silence is judged on the mailboxes its mail could reach once the
// run's own grade fails: the owner's, and those of every user whose mailbox
// captured mail on the deal. One of them not read leaves the deal unknown. A
// deal with no owner and no captured mail keeps the run's grade.
func TestADealsMailIsJudgedOnEveryMailboxItsMailReached(t *testing.T) {
	t.Parallel()
	healthy, colleague, broken, bare := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	byOwner := map[ids.UUID]assurance.SourceCoverage{
		healthy:   {Source: sourceMail, State: assurance.CoverageChecked},
		colleague: {Source: sourceMail, State: assurance.CoverageChecked},
		broken:    {Source: sourceMail, State: assurance.CoveragePermissionLimited},
	}
	cases := []struct {
		name       string
		runCovered bool
		owner      *ids.UUID
		mailUsers  []string
		want       bool
	}{
		{"every mailbox read, any owner", true, &broken, nil, true},
		{"run not read, owner's mailbox read", false, &healthy, nil, true},
		{"run not read, owner and colleague read", false, &healthy, []string{colleague.String(), healthy.String()}, true},
		{"run not read, a colleague on the deal is broken", false, &healthy, []string{broken.String()}, false},
		{"run not read, owner's mailbox broken", false, &broken, nil, false},
		{"run not read, owner has no mailbox", false, &bare, nil, false},
		{"run not read, a mail user has no mailbox", false, &healthy, []string{bare.String()}, false},
		{"run not read, no owner, colleague read", false, nil, []string{colleague.String()}, true},
		{"run not read, no owner, no mail", false, nil, nil, false},
		{"run not read, an id that does not parse", false, &healthy, []string{"not-a-uuid"}, false},
		{"every mailbox read, no owner", true, nil, nil, true},
	}
	for _, c := range cases {
		if got := dealMailCovered(c.runCovered, byOwner)(c.owner, c.mailUsers); got != c.want {
			t.Errorf("%s: covered = %v, want %v", c.name, got, c.want)
		}
	}
}

// The per-owner grading reads the same rules as the run's: a live mailbox
// never synced, or synced too long ago, is not checked.
func TestOneOwnersMailboxesAreGradedLikeTheRun(t *testing.T) {
	t.Parallel()
	now := clocktest.Now(t)
	recent, old := now.Add(-time.Hour), now.Add(-5*24*time.Hour)
	cases := []struct {
		name   string
		counts mailCounts
		want   string
	}{
		{"none live", mailCounts{}, assurance.CoverageNotConnected},
		{"needs re-auth", mailCounts{live: 1, reauth: 1, oldestSuccess: &recent}, assurance.CoveragePermissionLimited},
		{"never synced", mailCounts{live: 2, unsynced: 1, oldestSuccess: &recent}, assurance.CoverageUnavailable},
		{"behind", mailCounts{live: 1, oldestSuccess: &old}, assurance.CoverageStale},
		{"on time", mailCounts{live: 1, oldestSuccess: &recent}, assurance.CoverageChecked},
	}
	for _, c := range cases {
		if got := c.counts.grade(now).State; got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
