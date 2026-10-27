// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A reminder reaches where its mail reaches, which is the whole rule the
// statements around it carry out.
func TestAReminderReachesWhereItsMailReaches(t *testing.T) {
	if got := reminderReach(true); got != string(crmcontracts.ActivityAudienceWorkspace) {
		t.Errorf("the reminder of workspace mail reaches %q", got)
	}
	if got := reminderReach(false); got != string(crmcontracts.ActivityAudienceSelected) {
		t.Errorf("the reminder of limited mail reaches %q", got)
	}
}

// The membership follows the reach, and an unassigned reminder names nobody.
//
// A narrowed row with no member reaches no reader. The case is pinned here
// rather than left to the first caller that files an unassigned reminder.
func TestAReminderNamesItsAssigneeOnlyWhereThatIsTheReach(t *testing.T) {
	assignee := ids.NewV7()
	selected := string(crmcontracts.ActivityAudienceSelected)
	workspace := string(crmcontracts.ActivityAudienceWorkspace)

	narrowed := reminderMembers(selected, &assignee)
	if len(narrowed) != 1 || narrowed[0].SubjectID != assignee {
		t.Fatalf("a narrowed reminder names %d members, want its assignee alone", len(narrowed))
	}
	if narrowed[0].SubjectType != string(crmcontracts.AudienceMemberSubjectTypeUser) {
		t.Errorf("the member reads as %q, want a user", narrowed[0].SubjectType)
	}
	if open := reminderMembers(workspace, &assignee); len(open) != 0 {
		t.Errorf("a workspace reminder names %d members, want none", len(open))
	}
	if unassigned := reminderMembers(selected, nil); len(unassigned) != 0 {
		t.Errorf("an unassigned reminder names %d members, want none", len(unassigned))
	}
}

// A reminder already agreeing with its mail is left alone.
//
// The consumer runs on every audience event, so most calls concern a mail
// whose reminder already agrees. Moving it anyway would write an audit row
// for a change that did not happen.
func TestAReminderAgreeingWithItsMailIsLeftAlone(t *testing.T) {
	selected := string(crmcontracts.ActivityAudienceSelected)
	workspace := string(crmcontracts.ActivityAudienceWorkspace)

	for _, c := range []struct {
		name   string
		shared bool
		stored string
		want   string
		move   bool
	}{
		{"workspace mail, workspace reminder", true, workspace, workspace, false},
		{"limited mail, narrowed reminder", false, selected, selected, false},
		{"mail opened since filing", true, selected, workspace, true},
		{"mail narrowed since filing", false, workspace, selected, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			want, move := reminderMove(c.shared, c.stored)
			if want != c.want || move != c.move {
				t.Errorf("reminderMove(%t, %q) = %q, %t; want %q, %t",
					c.shared, c.stored, want, move, c.want, c.move)
			}
		})
	}
}

// The reminder this pass may move is open, unarchived and unrestricted.
//
// Each guard is asked because a row missing one is a row the pass must leave:
// a done reminder was answered, an archived one is off every lane, and a
// restricted one is under a statutory obligation.
func TestOnlyAnOpenUnarchivedReminderIsMoved(t *testing.T) {
	clause := openReminderClause("t")
	for _, guard := range []string{
		`"t".kind = 'task'`,
		`"t".archived_at IS NULL`,
		`NOT "t".is_done`,
		`"t".restricted_at IS NULL`,
	} {
		if !strings.Contains(clause, guard) {
			t.Errorf("the clause does not ask %s:\n%s", guard, clause)
		}
	}
}

// The alias is quoted, so a catalog name cannot be read as SQL.
func TestTheReminderClauseQuotesItsAlias(t *testing.T) {
	if clause := openReminderClause("task alias"); !strings.Contains(clause, `"task alias".kind`) {
		t.Errorf("the alias is not quoted:\n%s", clause)
	}
}
