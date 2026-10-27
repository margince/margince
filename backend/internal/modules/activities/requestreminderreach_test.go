// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
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
