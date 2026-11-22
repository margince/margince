// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datataskhealth

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	technical    = crmcontracts.HealthSourceConditionTechnical
	waitingHuman = crmcontracts.HealthSourceConditionWaitingHuman
	retrying     = crmcontracts.HealthSourceConditionRetrying
	processing   = crmcontracts.HealthSourceConditionProcessing
)

func TestUpdateHealthDistinguishesOperatorWorkFromAutomaticProgress(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later, earlier := now.Add(time.Minute), now.Add(-time.Minute)
	left := &datatask.ActionNeeded{Reason: "privacy_reviewer_missing", Count: 1}
	cases := []struct {
		name      string
		progress  datatask.Progress
		code      string
		condition crmcontracts.HealthSourceCondition
		included  bool
	}{
		{name: "completed", progress: datatask.Progress{Status: "done"}},
		{name: "completed with action needed", progress: datatask.Progress{Status: "done", Action: left}, code: "updates_action", condition: waitingHuman, included: true},
		{name: "unrequested manual task", progress: datatask.Progress{BlockedReason: "review_required"}},
		{name: "failed", progress: datatask.Progress{Status: "failed", NextAttemptAt: &later}, code: "updates_failed", condition: technical, included: true},
		{name: "paused", progress: datatask.Progress{Status: "paused"}, code: "updates_review", condition: waitingHuman, included: true},
		{name: "review requested", progress: datatask.Progress{Version: 1, BlockedReason: "review_required"}, code: "updates_review", condition: waitingHuman, included: true},
		{name: "missing prerequisite", progress: datatask.Progress{BlockedReason: "missing_prerequisite"}, code: "updates_review", condition: waitingHuman, included: true},
		{name: "newer release needed", progress: datatask.Progress{BlockedReason: "newer_release_required"}, code: "updates_review", condition: waitingHuman, included: true},
		{name: "scheduled retry", progress: datatask.Progress{Status: "pending", NextAttemptAt: &later}, code: "updates_retrying", condition: retrying, included: true},
		{name: "retry due", progress: datatask.Progress{Status: "pending", NextAttemptAt: &earlier}, code: "updates_processing", condition: processing, included: true},
		{name: "running", progress: datatask.Progress{Status: "running"}, code: "updates_processing", condition: processing, included: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, condition, included := Finding(tc.progress, now)
			if code != tc.code || condition != tc.condition || included != tc.included {
				t.Fatalf("got %q/%q included=%v; want %q/%q included=%v", code, condition, included, tc.code, tc.condition, tc.included)
			}
		})
	}
}

func TestAnActionNamesItsRecordsAndTellsTheReaderWhatToDo(t *testing.T) {
	record := ids.NewV7()
	got := Action(&datatask.ActionNeeded{
		Reason: "privacy_reviewer_missing", Count: 3,
		Records: []datatask.RecordRef{{Type: "approval_history", ID: record}},
	}, textlang.German, true)
	if got == nil || got.Count != 3 || len(got.Records) != 1 || got.Records[0].Type != "approval_history" || ids.UUID(got.Records[0].Id) != record {
		t.Fatalf("action=%+v", got)
	}
	english := Action(&datatask.ActionNeeded{Reason: "privacy_reviewer_missing"}, textlang.English, true)
	if got.Sentence == "" || got.Remedy == "" || got.Sentence == english.Sentence || got.Remedy == english.Remedy {
		t.Fatalf("the German reader got %q / %q", got.Sentence, got.Remedy)
	}
	unknown := Action(&datatask.ActionNeeded{Reason: "reason_this_build_lacks"}, textlang.English, true)
	if unknown.Sentence == "" || unknown.Remedy == "" || unknown.Sentence == english.Sentence {
		t.Fatalf("an unknown reason read %q / %q", unknown.Sentence, unknown.Remedy)
	}
	if Action(nil, textlang.English, true) != nil {
		t.Fatal("a step with no action showed one")
	}
}

func TestOnlyAReaderWithReviewerAccessSeesTheRecords(t *testing.T) {
	reviewer := principal.Principal{Type: principal.PrincipalHuman, ID: "user:reviewer", Permissions: principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"audit_log": {Read: true}, "data_task": {Read: true}},
		RowScope: principal.RowScopeAll,
	}}
	ownRows := reviewer
	ownRows.Permissions.RowScope = principal.RowScopeOwn
	masked := reviewer
	masked.Permissions.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "margin", Condition: principal.MaskAlways}}
	noAudit := reviewer
	noAudit.Permissions.Objects = map[string]principal.ObjectGrant{"data_task": {Read: true}}
	for name, c := range map[string]struct {
		who  principal.Principal
		want bool
	}{"reviewer": {reviewer, true}, "own rows only": {ownRows, false}, "field masks": {masked, false}, "no audit read": {noAudit, false}} {
		if got := MaySeeRecords(principal.WithActor(t.Context(), c.who)); got != c.want {
			t.Errorf("%s: may see records = %v, want %v", name, got, c.want)
		}
	}
	left := &datatask.ActionNeeded{Reason: "privacy_reviewer_missing", Count: 1, Records: []datatask.RecordRef{{Type: "approval_history", ID: ids.NewV7()}}}
	if counted := Action(left, textlang.English, false); counted.Count != 1 || len(counted.Records) != 0 {
		t.Fatalf("a reader without reviewer access got %+v", counted)
	}
}
