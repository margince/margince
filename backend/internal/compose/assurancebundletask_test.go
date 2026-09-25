// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The three answers the task's lifecycle reaches without a database: what the
// task is called, whether a refusal was about the SEAT rather than the task,
// and whether a task still names the subject it was raised for.

import (
	"errors"
	"fmt"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The subject line is what a rep reads on their list, and one finding is the
// ordinary case — a plural there reads as a template nobody finished.
func TestTheTaskCountsItsFindingsInWords(t *testing.T) {
	t.Parallel()

	for findings, want := range map[int]string{
		1: "1 forecast input needs attention",
		2: "2 forecast inputs need attention",
		7: "7 forecast inputs need attention",
	} {
		if got := bundleTaskSubject(findings); got != want {
			t.Errorf("bundleTaskSubject(%d) = %q, want %q", findings, got, want)
		}
	}
}

// A refusal about WHO the task was aimed at is retried unassigned; anything
// else is a real failure. Retrying the wrong one would mint a task nobody is
// asked to do out of an error that had nothing to do with the assignee.
func TestOnlyASeatRefusalIsRetriedUnassigned(t *testing.T) {
	t.Parallel()

	// A seat that is missing, inactive or archived — deliberately one error.
	if !unassignableSeat(fmt.Errorf("wrapped: %w", apperrors.ErrNotFound)) {
		t.Error("a missing or inactive seat did not read as a seat refusal, so the deals of " +
			"everyone who has left stop being checked")
	}
	if !unassignableSeat(&activities.AgentAssigneeError{}) {
		t.Error("an agent seat did not read as a seat refusal")
	}
	if unassignableSeat(apperrors.ErrPermissionDenied) {
		t.Error("a permission refusal read as a seat refusal — it would be retried into a " +
			"task nobody was asked to do, out of an error about something else")
	}
	if unassignableSeat(errors.New("the database went away")) {
		t.Error("an ordinary failure read as a seat refusal")
	}
}

// A task relinked to another record is not this subject's any more, and
// adopting it would hang one deal's findings off another's row.
func TestATaskNamesTheSubjectOrItIsNotAdopted(t *testing.T) {
	t.Parallel()

	deal := ids.NewV7()
	subject := subjectFindings{kind: "deal", id: deal}
	linked := func(kind string, id ids.UUID) crmcontracts.Activity {
		links := []crmcontracts.ActivityLink{{
			EntityType: crmcontracts.ActivityLinkEntityType(kind),
			EntityId:   openapi_types.UUID(id),
		}}
		return crmcontracts.Activity{Links: &links}
	}

	if !linksToSubject(linked("deal", deal), subject) {
		t.Error("a task linked to its own deal did not read as the subject's")
	}
	if linksToSubject(linked("deal", ids.NewV7()), subject) {
		t.Error("a task relinked to ANOTHER deal read as this subject's — adopting it would " +
			"hang one deal's findings off the other's row")
	}
	if linksToSubject(linked("company", deal), subject) {
		t.Error("a link of a different kind carrying the same id read as the subject's")
	}
	// A task with no links at all: nothing says it is still about this deal.
	if linksToSubject(crmcontracts.Activity{}, subject) {
		t.Error("a task naming nothing read as the subject's")
	}
}
