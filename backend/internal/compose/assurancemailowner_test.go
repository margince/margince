// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A deal's silence is judged on its owner's mailboxes once the run's own grade
// fails. An owner with no mailbox, or a deal with no owner, keeps the run's
// grade, which then fails closed.
func TestADealsMailIsJudgedOnItsOwnersMailboxes(t *testing.T) {
	t.Parallel()
	healthy, broken, bare := ids.NewV7(), ids.NewV7(), ids.NewV7()
	byOwner := map[ids.UUID]assurance.SourceCoverage{
		healthy: {Source: sourceMail, State: assurance.CoverageChecked},
		broken:  {Source: sourceMail, State: assurance.CoveragePermissionLimited},
	}
	cases := []struct {
		name       string
		runCovered bool
		owner      *ids.UUID
		want       bool
	}{
		{"every mailbox read, any owner", true, &broken, true},
		{"run not read, owner's mailbox read", false, &healthy, true},
		{"run not read, owner's mailbox broken", false, &broken, false},
		{"run not read, owner has no mailbox", false, &bare, false},
		{"run not read, no owner", false, nil, false},
		{"every mailbox read, no owner", true, nil, true},
	}
	for _, c := range cases {
		if got := dealMailCovered(c.runCovered, byOwner)(c.owner); got != c.want {
			t.Errorf("%s: covered = %v, want %v", c.name, got, c.want)
		}
	}
}

// The per-owner grading reads the same rules as the run's: a live mailbox
// never synced, or synced too long ago, is not checked.
func TestOneOwnersMailboxesAreGradedLikeTheRun(t *testing.T) {
	t.Parallel()
	now := time.Now()
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
