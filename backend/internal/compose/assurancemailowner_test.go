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

// A deal's silence is judged on its own mailboxes, whatever the run's grade
// says: the owner's live connections and each connection that delivered mail
// linked to the deal. A delivering connection that is gone, rebound to another
// account, or not read on time leaves the deal unknown. A deal with no owner and
// no captured mail keeps the run's grade.
func TestADealsMailIsJudgedOnTheConnectionsThatDeliveredIt(t *testing.T) {
	t.Parallel()
	healthy, colleague, broken, bare := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	checked := assurance.SourceCoverage{Source: sourceMail, State: assurance.CoverageChecked}
	limited := assurance.SourceCoverage{Source: sourceMail, State: assurance.CoveragePermissionLimited}
	byOwner := map[ids.UUID]assurance.SourceCoverage{healthy: checked, colleague: checked, broken: limited}
	byConnection := map[mailConnection]connectionCoverage{
		{user: healthy, provider: "gmail"}:   {generation: "1", coverage: checked},
		{user: colleague, provider: "graph"}: {generation: "2", coverage: checked},
		{user: broken, provider: "imap"}:     {generation: "0", coverage: limited},
	}
	via := func(user ids.UUID, mailbox string) string { return user.String() + ":" + mailbox }
	cases := []struct {
		name       string
		runCovered bool
		owner      *ids.UUID
		mailboxes  []string
		want       bool
	}{
		{"owner read, no captured mail", false, &healthy, nil, true},
		{"owner read, delivered through a read connection", false, &healthy, []string{via(colleague, "graph:2")}, true},
		{"run checked does not cover an owner with no mailbox", true, &bare, nil, false},
		{"run checked does not cover a gone delivering connection", true, &healthy, []string{via(colleague, "gmail:2")}, false},
		{"a delivering connection rebound to another account", false, &healthy, []string{via(colleague, "graph:1")}, false},
		{"a delivering connection not read on time", false, &healthy, []string{via(broken, "imap:0")}, false},
		{"owner's mailbox broken", false, &broken, nil, false},
		{"an import naming no connection, user read", false, &healthy, []string{colleague.String()}, true},
		{"an import naming no connection, user without mailbox", false, &healthy, []string{bare.String()}, false},
		{"no owner, delivered through a read connection", false, nil, []string{via(healthy, "gmail:1")}, true},
		{"no owner, no mail, run checked", true, nil, nil, true},
		{"no owner, no mail, run not checked", false, nil, nil, false},
		{"an entry that does not parse", false, &healthy, []string{"not-a-uuid:gmail:1"}, false},
		{"a mailbox entry without a generation", false, &healthy, []string{via(healthy, "gmail")}, false},
	}
	for _, c := range cases {
		if got := dealMailCovered(c.runCovered, byOwner, byConnection)(c.owner, c.mailboxes); got != c.want {
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
