// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

// A reminder already agreeing with its mail is left alone, and an unassigned
// one is not narrowed.
//
// The consumer runs on every audience event, so most calls concern a mail
// whose reminder already agrees. Moving it anyway would write an audit row
// for a change that did not happen. A narrowed reach names the assignee as
// its reader, so with no assignee it would reach nobody.
func TestAReminderAgreeingWithItsMailIsLeftAlone(t *testing.T) {
	selected := string(crmcontracts.ActivityAudienceSelected)
	workspace := string(crmcontracts.ActivityAudienceWorkspace)
	assignee := ids.NewV7()

	for _, c := range []struct {
		name     string
		shared   bool
		stored   string
		assignee *ids.UUID
		want     string
		move     bool
	}{
		{"workspace mail, workspace reminder", true, workspace, &assignee, workspace, false},
		{"limited mail, narrowed reminder", false, selected, &assignee, selected, false},
		{"mail opened since filing", true, selected, &assignee, workspace, true},
		{"mail narrowed since filing", false, workspace, &assignee, selected, true},
		{"mail narrowed, nobody assigned", false, workspace, nil, workspace, false},
		{"mail opened, nobody assigned", true, selected, nil, workspace, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			want, move := reminderMove(c.shared, c.stored, c.assignee)
			if want != c.want || move != c.move {
				t.Errorf("reminderMove(%t, %q, assigned %t) = %q, %t; want %q, %t",
					c.shared, c.stored, c.assignee != nil, want, move, c.want, c.move)
			}
		})
	}
}

// The reminder this pass may move is open, unarchived and unrestricted.
//
// A row missing any guard is a row the pass must leave.
//
// A done reminder was answered. An archived one is off every lane. A
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

// A caller with no standing moves no reminder.
//
// The guard runs before the transaction is touched.
//
// That is what lets this test pass a nil one. A refusal reaching the database
// would be a refusal that had already done work.
func TestOnlyTheSystemMovesAReminder(t *testing.T) {
	err := RecomputeRequestReminderReachTx(context.Background(), nil, ids.ActivityID{})
	if err == nil {
		t.Fatal("a caller with no standing moved a reminder")
	}
	// Either shape is the caller's standing. A bound principal that is not the
	// system is denied; an unbound one has no standing at all.
	if !errors.Is(err, apperrors.ErrPermissionDenied) && !strings.Contains(err.Error(), "actor") {
		t.Errorf("the refusal reads %v, want it about the caller's standing", err)
	}
}
