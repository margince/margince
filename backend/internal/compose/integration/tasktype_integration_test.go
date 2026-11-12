// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A task's type on every read, and the one step it allows. Only a reply or a
// reach out offers email. A reply names the way its message came in.

import (
	"errors"
	"net/http"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (s *standingEnv) typedTask(t *testing.T, taskType crmcontracts.TaskType, link activities.ActivityLinkInput) ids.UUID {
	t.Helper()
	subject := "Call Carol back about the delivery date"
	logged, _, err := s.Activities.LogActivity(s.As(s.Rep1, []ids.UUID{s.Team1}, activityLifecyclePerms),
		activities.LogActivityInput{Kind: "task", Subject: &subject, TaskType: &taskType, Links: []activities.ActivityLinkInput{link}})
	if err != nil {
		t.Fatalf("logging the %s task: %v", taskType, err)
	}
	return ids.UUID(logged.Id)
}

func (s *standingEnv) readTask(t *testing.T, row ids.UUID) crmcontracts.Activity {
	t.Helper()
	code, body := serve(s.reader(), t, func(w http.ResponseWriter, r *http.Request) {
		s.replyHandlers().GetActivity(w, r, crmcontracts.Id(row))
	})
	if code != http.StatusOK {
		t.Fatalf("GET /activities/{id} answered %d", code)
	}
	return readBody[crmcontracts.Activity](t, body)
}

func (s *standingEnv) readTaskType(t *testing.T, row ids.UUID) crmcontracts.TaskType {
	t.Helper()
	got := s.readTask(t, row).TaskType
	if got == nil {
		t.Fatal("a task reads no task_type")
	}
	return *got
}

func TestATaskReadsTheTypeItWasGivenAndOnlyAMailTypeOffersEmail(t *testing.T) {
	s := setupStanding(t)
	for _, tc := range []struct {
		taskType crmcontracts.TaskType
		offers   bool
	}{
		{crmcontracts.TaskTypeReply, true},
		{crmcontracts.TaskTypeReachOut, true},
		{crmcontracts.TaskTypeMeet, false},
		{crmcontracts.TaskTypeReview, false},
		{crmcontracts.TaskTypeDecide, false},
		{crmcontracts.TaskTypeDo, false},
	} {
		t.Run(string(tc.taskType), func(t *testing.T) {
			row := s.typedTask(t, tc.taskType, s.contactAt(t, "Carol "+string(tc.taskType), string(tc.taskType)+"@buyer.test"))
			if got := s.readTaskType(t, row); got != tc.taskType {
				t.Errorf("task_type = %q, want %q", got, tc.taskType)
			}
			if offers, _ := s.offeredOnEveryRead(t, row); offers != tc.offers {
				t.Errorf("offers_email = %v, want %v", offers, tc.offers)
			}
		})
	}
}

func TestAnUntypedTaskReadsTheTypeOfWhatMadeIt(t *testing.T) {
	s := setupStanding(t)
	for _, tc := range []struct {
		name, origin, source string
		want                 crmcontracts.TaskType
	}{
		{"a task a rep wrote", "human", "", crmcontracts.TaskTypeReachOut},
		{"an answer owed to a request", "system_remediation", "email_request", crmcontracts.TaskTypeReply},
		{"a lead to qualify", "system_remediation", "lead_replied", crmcontracts.TaskTypeReview},
		{"a mail to file under a deal", "system_remediation", "which_deal", crmcontracts.TaskTypeDecide},
		{"a notice the product filed", "system_notice", "", crmcontracts.TaskTypeDo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := s.writtenAbout(t, "task", s.contactAt(t, "Dana Vogt", ""))
			s.WsExec(t, `UPDATE activity SET origin = $2, source_system = NULLIF($3, ''), source_id = id::text WHERE id = $1`,
				row, tc.origin, tc.source)
			if got := s.readTaskType(t, row); got != tc.want {
				t.Errorf("task_type = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAPatchRetypesATaskAndATypeOnAnotherKindIsRefused(t *testing.T) {
	s := setupStanding(t)
	ctx := s.As(s.Rep1, []ids.UUID{s.Team1}, activityLifecyclePerms)
	row := s.typedTask(t, crmcontracts.TaskTypeReachOut, s.contactAt(t, "Emil Roth", "emil@buyer.test"))
	meet := crmcontracts.TaskTypeMeet
	if _, err := s.Activities.UpdateActivity(ctx, ids.From[ids.ActivityKind](row), activities.UpdateActivityInput{TaskType: &meet}); err != nil {
		t.Fatalf("retyping the task: %v", err)
	}
	if got := s.readTaskType(t, row); got != meet {
		t.Errorf("task_type = %q after the patch, want meet", got)
	}
	subject := "Met at the fair"
	_, _, err := s.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: &subject, TaskType: &meet})
	var refused *activities.KindFieldError
	if !errors.As(err, &refused) || refused.Field != "task_type" {
		t.Errorf("a note with a task type: err = %v, want a task_type kind refusal", err)
	}
}

func TestAReplyNamesTheWayItsMessageCameIn(t *testing.T) {
	s := setupStanding(t)
	for _, tc := range []struct {
		name, sourceKind string
		taskType         crmcontracts.TaskType
		want             *crmcontracts.ActivityReplyChannel
	}{
		{"a reply to a call", "call", crmcontracts.TaskTypeReply, new(crmcontracts.ActivityReplyChannelCall)},
		{"a reply to a note", "note", crmcontracts.TaskTypeReply, nil},
		{"a reply with no source", "", crmcontracts.TaskTypeReply, nil},
		{"a reach out after a call", "call", crmcontracts.TaskTypeReachOut, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			contact := s.contactAt(t, "Ida Brandt", "")
			row := s.typedTask(t, tc.taskType, contact)
			if tc.sourceKind != "" {
				source := s.writtenAbout(t, tc.sourceKind, contact)
				s.WsExec(t, `UPDATE activity SET source_activity_id = $2 WHERE id = $1`, row, source)
			}
			got := s.readTask(t, row).ReplyChannel
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Errorf("reply_channel = %v, want %v", got, tc.want)
			}
		})
	}
}
